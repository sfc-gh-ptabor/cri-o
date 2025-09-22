package imageprovider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	"github.com/opencontainers/go-digest"
)

const (
	// RootFSScheme is the URL scheme for root filesystem images
	RootFSScheme = "root-fs"
	
	// RootFSProviderName is the name of the root filesystem provider
	RootFSProviderName = "root-fs"
)

// RootFSProvider implements ImageProvider for root filesystem directories
// It handles image references like: root-fs:///opt/my-containers/my-image
type RootFSProvider struct {
	// allowedPaths contains the list of allowed base paths for security
	allowedPaths []string
}

// NewRootFSProvider creates a new root filesystem image provider
func NewRootFSProvider(allowedPaths []string) *RootFSProvider {
	return &RootFSProvider{
		allowedPaths: allowedPaths,
	}
}

// Name returns the provider name
func (p *RootFSProvider) Name() string {
	return RootFSProviderName
}

// CanHandle checks if this provider can handle the given image spec
func (p *RootFSProvider) CanHandle(imageSpec *criv1.ImageSpec) bool {
	if imageSpec == nil {
		return false
	}
	
	// Check if the image reference uses the root-fs scheme
	image := imageSpec.GetImage()
	return strings.HasPrefix(image, RootFSScheme+"://")
}

// PullImage "pulls" a root filesystem image by validating the directory exists
func (p *RootFSProvider) PullImage(ctx context.Context, imageSpec *criv1.ImageSpec, options *ImageProviderOptions) (*ImageProviderResult, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	
	rootfsPath, err := p.parseAndValidateImageRef(imageSpec.GetImage())
	if err != nil {
		return nil, fmt.Errorf("invalid root-fs image reference: %w", err)
	}
	
	log.Infof(ctx, "Using root filesystem from: %s", rootfsPath)
	
	// Verify the path exists and is a directory
	info, err := os.Stat(rootfsPath)
	if err != nil {
		return nil, fmt.Errorf("root filesystem path does not exist: %s: %w", rootfsPath, err)
	}
	
	if !info.IsDir() {
		return nil, fmt.Errorf("root filesystem path is not a directory: %s", rootfsPath)
	}
	
	// Check if path is allowed
	if !p.isPathAllowed(rootfsPath) {
		return nil, fmt.Errorf("root filesystem path not allowed: %s", rootfsPath)
	}
	
	// Create a storage reference that points to the root filesystem
	// We'll use a special format that the storage layer can understand
	imageRef, err := p.createImageReference(imageSpec.GetImage(), rootfsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create image reference: %w", err)
	}
	
	// Calculate directory size (optional)
	size := p.calculateDirectorySize(rootfsPath)
	
	// Create digest from path for uniqueness
	pathDigest := digest.FromString(rootfsPath)
	
	return &ImageProviderResult{
		ImageRef: imageRef,
		Size:     size,
		Digest:   pathDigest.String(),
		Labels:   map[string]string{
			"io.cri-o.image.provider": RootFSProviderName,
		},
		Metadata: map[string]string{
			"rootfs.path": rootfsPath,
			"rootfs.original_ref": imageSpec.GetImage(),
		},
	}, nil
}

// ImageStatus returns the status of a root filesystem image
func (p *RootFSProvider) ImageStatus(ctx context.Context, imageSpec *criv1.ImageSpec) (*ImageProviderResult, error) {
	// For root-fs images, status is the same as pull (just validation)
	return p.PullImage(ctx, imageSpec, nil)
}

// RemoveImage removes a root filesystem image (no-op since it's just a directory)
func (p *RootFSProvider) RemoveImage(ctx context.Context, imageSpec *criv1.ImageSpec) error {
	// Root filesystem images are not "removed" since they're just directories
	// We could potentially track them and "unregister" but for simplicity, this is a no-op
	log.Infof(ctx, "Root filesystem image removal requested for: %s (no-op)", imageSpec.GetImage())
	return nil
}

// ListImages returns all root filesystem images (not implemented for directories)
func (p *RootFSProvider) ListImages(ctx context.Context) ([]*ImageProviderResult, error) {
	// For directory-based images, listing is not straightforward
	// Return empty list for now
	return []*ImageProviderResult{}, nil
}

// parseAndValidateImageRef parses a root-fs:// URL and returns the filesystem path
func (p *RootFSProvider) parseAndValidateImageRef(imageRef string) (string, error) {
	if !strings.HasPrefix(imageRef, RootFSScheme+"://") {
		return "", fmt.Errorf("invalid scheme, expected %s://", RootFSScheme)
	}
	
	// Remove the scheme prefix
	path := strings.TrimPrefix(imageRef, RootFSScheme+"://")
	
	if path == "" {
		return "", errors.New("empty path in root-fs image reference")
	}
	
	// Clean the path
	path = filepath.Clean(path)
	
	// Ensure absolute path
	if !filepath.IsAbs(path) {
		return "", errors.New("root filesystem path must be absolute")
	}
	
	return path, nil
}

// isPathAllowed checks if the given path is under an allowed base path
func (p *RootFSProvider) isPathAllowed(path string) bool {
	if len(p.allowedPaths) == 0 {
		// If no restrictions configured, allow any absolute path
		return true
	}
	
	cleanPath := filepath.Clean(path)
	
	for _, allowedPath := range p.allowedPaths {
		cleanAllowedPath := filepath.Clean(allowedPath)
		
		// Check if the path is under the allowed path
		if strings.HasPrefix(cleanPath, cleanAllowedPath) {
			return true
		}
	}
	
	return false
}

// calculateDirectorySize calculates the total size of a directory
func (p *RootFSProvider) calculateDirectorySize(dirPath string) *uint64 {
	var totalSize uint64
	
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += uint64(info.Size())
		}
		return nil
	})
	
	if err != nil {
		// If we can't calculate size, return nil
		return nil
	}
	
	return &totalSize
}

// createImageReference creates a storage reference for the root filesystem image
func (p *RootFSProvider) createImageReference(originalRef, rootfsPath string) (storage.RegistryImageReference, error) {
	// For root-fs images, we'll create a special reference that includes metadata
	// The reference format will be: root-fs-provider://<path-hash>@<digest>
	
	pathDigest := digest.FromString(rootfsPath)
	
	// Create a reference that embeds the path information
	// This allows the storage layer to know it's a root-fs image
	refString := fmt.Sprintf("root-fs-provider://%s@%s", 
		digest.FromString(originalRef).Encoded()[:12], // Short hash of original ref
		pathDigest)
	
	imageRef, err := references.ParseRegistryImageReferenceFromOutOfProcessData(refString)
	if err != nil {
		return storage.RegistryImageReference{}, fmt.Errorf("failed to parse root-fs image reference: %w", err)
	}
	
	return imageRef, nil
}