package imageprovider

import (
	"context"

	"github.com/cri-o/cri-o/internal/storage"
	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// DefaultProvider wraps the existing CRI-O image service functionality
// It handles standard container images (docker://, containers-storage://, etc.)
type DefaultProvider struct {
	imageService storage.ImageServer
}

// NewDefaultProvider creates a new default image provider
func NewDefaultProvider(imageService storage.ImageServer) *DefaultProvider {
	return &DefaultProvider{
		imageService: imageService,
	}
}

// Name returns the provider name
func (p *DefaultProvider) Name() string {
	return "default"
}

// CanHandle checks if this provider can handle the given image spec
// The default provider handles everything that doesn't have a specific provider
func (p *DefaultProvider) CanHandle(imageSpec *criv1.ImageSpec) bool {
	// Default provider can handle any image spec
	return true
}

// PullImage pulls an image using the existing image service
func (p *DefaultProvider) PullImage(ctx context.Context, imageSpec *criv1.ImageSpec, options *ImageProviderOptions) (*ImageProviderResult, error) {
	// Convert ImageSpec to internal format
	imageRef, err := parseImageSpecToReference(imageSpec)
	if err != nil {
		return nil, err
	}
	
	// Convert options to internal format
	copyOptions := &storage.ImageCopyOptions{}
	if options != nil {
		copyOptions = convertToImageCopyOptions(options)
	}
	
	// Pull the image using existing functionality
	resultRef, err := p.imageService.PullImage(ctx, imageRef, copyOptions)
	if err != nil {
		return nil, err
	}
	
	// Get image status to fill in additional details
	imageResult, err := p.imageService.ImageStatusByName(copyOptions.SourceCtx, resultRef)
	if err != nil {
		return nil, err
	}
	
	return &ImageProviderResult{
		ImageRef: resultRef,
		Size:     imageResult.Size,
		Digest:   imageResult.Digest.String(),
		Labels:   imageResult.Labels,
		Metadata: map[string]string{
			"provider": "default",
		},
	}, nil
}

// ImageStatus returns the status of an image using the existing image service
func (p *DefaultProvider) ImageStatus(ctx context.Context, imageSpec *criv1.ImageSpec) (*ImageProviderResult, error) {
	imageRef, err := parseImageSpecToReference(imageSpec)
	if err != nil {
		return nil, err
	}
	
	imageResult, err := p.imageService.ImageStatusByName(nil, imageRef)
	if err != nil {
		return nil, err
	}
	
	return &ImageProviderResult{
		ImageRef: *imageResult.SomeNameOfThisImage,
		Size:     imageResult.Size,
		Digest:   imageResult.Digest.String(),
		Labels:   imageResult.Labels,
		Metadata: map[string]string{
			"provider": "default",
		},
	}, nil
}

// RemoveImage removes an image using the existing image service
func (p *DefaultProvider) RemoveImage(ctx context.Context, imageSpec *criv1.ImageSpec) error {
	imageRef, err := parseImageSpecToReference(imageSpec)
	if err != nil {
		return err
	}
	
	return p.imageService.UntagImage(nil, imageRef)
}

// ListImages returns all images using the existing image service
func (p *DefaultProvider) ListImages(ctx context.Context) ([]*ImageProviderResult, error) {
	images, err := p.imageService.ListImages(nil)
	if err != nil {
		return nil, err
	}
	
	results := make([]*ImageProviderResult, 0, len(images))
	for _, img := range images {
		if img.SomeNameOfThisImage != nil {
			results = append(results, &ImageProviderResult{
				ImageRef: *img.SomeNameOfThisImage,
				Size:     img.Size,
				Digest:   img.Digest.String(),
				Labels:   img.Labels,
				Metadata: map[string]string{
					"provider": "default",
				},
			})
		}
	}
	
	return results, nil
}