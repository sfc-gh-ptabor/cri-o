package server

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	istorage "github.com/containers/image/v5/storage"
	"github.com/containers/storage"
	json "github.com/json-iterator/go"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/imageprovider"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/ociartifact"
	pkgstorage "github.com/cri-o/cri-o/internal/storage"
)

// ImageStatus returns the status of the image.
func (s *Server) ImageStatus(ctx context.Context, req *types.ImageStatusRequest) (*types.ImageStatusResponse, error) {
	_ = (*imageprovider.Service)(nil) // Force import usage
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	img := req.GetImage()
	if img == nil || img.GetImage() == "" {
		return nil, errors.New("no image specified")
	}

	log.Infof(ctx, "Checking image status: %s", img.GetImage())

	// Use image provider service if available
	if s.imageProviderService != nil {
		return s.imageStatusWithProviders(ctx, req)
	}

	// Legacy path
	status, err := s.storageImageStatus(ctx, img)
	if err != nil {
		return nil, err
	}

	if status == nil {
		artifact, err := s.ArtifactStore().Status(ctx, img.GetImage())
		if err == nil {
			return &types.ImageStatusResponse{
				Image: artifact.CRIImage(),
			}, nil
		}

		if errors.Is(err, ociartifact.ErrNotFound) {
			log.Infof(ctx, "Neither image nor artfiact %s found", img.GetImage())
		} else if err != nil {
			log.Errorf(ctx, "Unable to get artifact: %v", err)
		}

		return &types.ImageStatusResponse{}, nil
	}

	// Ensure that size is already defined
	var size uint64
	if status.Size == nil {
		size = 0
	} else {
		size = *status.Size
	}

	resp := &types.ImageStatusResponse{
		Image: &types.Image{
			Id:          status.ID.IDStringForOutOfProcessConsumptionOnly(),
			RepoTags:    status.RepoTags,
			RepoDigests: status.RepoDigests,
			Size:        size,
			Spec: &types.ImageSpec{
				Annotations: status.Annotations,
			},
			Pinned: status.Pinned,
		},
	}

	if req.GetVerbose() {
		info, err := createImageInfo(status)
		if err != nil {
			return nil, fmt.Errorf("creating image info: %w", err)
		}

		resp.Info = info
	}

	uid, username := getUserFromImage(status.User)
	if uid != nil {
		resp.Image.Uid = &types.Int64Value{Value: *uid}
	}

	resp.Image.Username = username

	return resp, nil
}

// storageImageStatus calls ImageStatus for a k8s ImageSpec.
// Returns (nil, nil) if image was not found.
func (s *Server) storageImageStatus(ctx context.Context, spec *types.ImageSpec) (*pkgstorage.ImageResult, error) {
	if id := s.ContainerServer.StorageImageServer().HeuristicallyTryResolvingStringAsIDPrefix(spec.GetImage()); id != nil {
		status, err := s.ContainerServer.StorageImageServer().ImageStatusByID(s.config.SystemContext, *id)
		if err != nil {
			if errors.Is(err, istorage.ErrNoSuchImage) || errors.Is(err, storage.ErrImageUnknown) {
				log.Infof(ctx, "Image %s not found", spec.GetImage())

				return nil, nil
			}

			log.Warnf(ctx, "Error getting status from %s: %v", spec.GetImage(), err)

			return nil, err
		}

		return status, nil
	}

	potentialMatches, err := s.ContainerServer.StorageImageServer().CandidatesForPotentiallyShortImageName(s.config.SystemContext, spec.GetImage())
	if err != nil {
		return nil, err
	}

	var lastErr error

	for _, name := range potentialMatches {
		status, err := s.ContainerServer.StorageImageServer().ImageStatusByName(s.config.SystemContext, name)
		if err != nil {
			if errors.Is(err, istorage.ErrNoSuchImage) {
				log.Debugf(ctx, "Can't find %s", name)

				continue
			}

			log.Warnf(ctx, "Error getting status from %s: %v", name, err)
			lastErr = err

			continue
		}

		return status, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	// CandidatesForPotentiallyShortImageName returns at least one value if it doesn't fail.
	// So, if we got here, there was at least one ErrNoSuchImage, and no other errors.
	log.Infof(ctx, "Image %s not found", spec.GetImage())

	return nil, nil
}

// getUserFromImage gets uid or user name of the image user.
// If user is numeric, it will be treated as uid; or else, it is treated as user name.
func getUserFromImage(user string) (id *int64, username string) {
	// return both empty if user is not specified in the image.
	if user == "" {
		return nil, ""
	}
	// split instances where the id may contain user:group
	user = strings.Split(user, ":")[0]
	// user could be either uid or user name. Try to interpret as numeric uid.
	uid, err := strconv.ParseInt(user, 10, 64)
	if err != nil {
		// If user is non numeric, assume it's user name.
		return nil, user
	}
	// If user is a numeric uid.
	return &uid, ""
}

func createImageInfo(result *pkgstorage.ImageResult) (map[string]string, error) {
	info := struct {
		Labels    map[string]string `json:"labels,omitempty"`
		ImageSpec *specs.Image      `json:"imageSpec"`
	}{
		result.Labels,
		result.OCIConfig,
	}

	bytes, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %v: %w", info, err)
	}

	return map[string]string{"info": string(bytes)}, nil
}

// imageStatusWithProviders uses the pluggable image provider system for image status
func (s *Server) imageStatusWithProviders(ctx context.Context, req *types.ImageStatusRequest) (*types.ImageStatusResponse, error) {
	img := req.GetImage()
	
	// Try to get status using image provider service
	result, err := s.imageProviderService.ImageStatus(ctx, img)
	if err != nil {
		// Fall back to legacy behavior for compatibility
		return s.imageStatusLegacy(ctx, req)
	}
	
	// Convert provider result to CRI response
	response := &types.ImageStatusResponse{
		Image: &types.Image{
			Id:          result.Digest,
			RepoTags:    []string{img.GetImage()},
			RepoDigests: []string{result.ImageRef.StringForOutOfProcessConsumptionOnly()},
			Size:        0, // Size will be set below if available
		},
	}
	
	if result.Size != nil {
		response.Image.Size = *result.Size
	}
	
	// Add verbose info if requested
	if req.GetVerbose() {
		info := map[string]string{
			"provider": "pluggable",
		}
		
		// Add metadata from provider
		for k, v := range result.Metadata {
			info[k] = v
		}
		
		response.Info = info
	}
	
	return response, nil
}

// imageStatusLegacy provides the original image status behavior
func (s *Server) imageStatusLegacy(ctx context.Context, req *types.ImageStatusRequest) (*types.ImageStatusResponse, error) {
	img := req.GetImage()
	
	status, err := s.storageImageStatus(ctx, img)
	if err != nil {
		return nil, err
	}

	if status == nil {
		artifact, err := s.ArtifactStore().Status(ctx, img.GetImage())
		if err == nil {
			return &types.ImageStatusResponse{
				Image: artifact.CRIImage(),
			}, nil
		}

		if errors.Is(err, ociartifact.ErrNotFound) {
			log.Infof(ctx, "Neither image nor artifact %s found", img.GetImage())
		} else if err != nil {
			log.Errorf(ctx, "Unable to get artifact: %v", err)
		}

		return &types.ImageStatusResponse{}, nil
	}

	imageID, ref := status.ID.IDStringForOutOfProcessConsumptionOnly(), status.RepoDigests
	if len(status.RepoTags) == 0 && len(status.RepoDigests) == 0 {
		// In this case set the image ID to the ID itself with digest prefix and the ref to empty.
		imageID = storage.ImageDigestBigDataKey + ":" + status.ID.IDStringForOutOfProcessConsumptionOnly()
		ref = []string{}
	}

	var UID *int64
	var username string
	if status.User != "" {
		UID, username = getUserFromImage(status.User)
	}

	response := &types.ImageStatusResponse{
		Image: &types.Image{
			Id:          imageID,
			RepoTags:    status.RepoTags,
			RepoDigests: ref,
			Size:        0,
			Uid:         &types.Int64Value{Value: 0},
			Username:    username,
			Spec:        img,
		},
	}
	if status.Size != nil {
		response.Image.Size = *status.Size
	}
	if UID != nil {
		response.Image.Uid.Value = *UID
	}

	if req.GetVerbose() {
		info, err := createImageInfo(status)
		if err != nil {
			return nil, fmt.Errorf("create image info: %w", err)
		}
		response.Info = info
	}

	return response, nil
}
