package imageprovider

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	imageTypes "github.com/containers/image/v5/types"
	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// parseImageSpecToReference converts a CRI ImageSpec to internal RegistryImageReference
func parseImageSpecToReference(imageSpec *criv1.ImageSpec) (storage.RegistryImageReference, error) {
	if imageSpec == nil {
		return storage.RegistryImageReference{}, fmt.Errorf("imageSpec cannot be nil")
	}
	
	image := imageSpec.GetImage()
	if image == "" {
		return storage.RegistryImageReference{}, fmt.Errorf("image field cannot be empty")
	}
	
	// Parse the image reference using the existing CRI-O logic
	imageRef, err := references.ParseRegistryImageReferenceFromOutOfProcessData(image)
	if err != nil {
		return storage.RegistryImageReference{}, fmt.Errorf("failed to parse image reference %q: %w", image, err)
	}
	
	return imageRef, nil
}

// convertToImageCopyOptions converts ImageProviderOptions to storage.ImageCopyOptions
func convertToImageCopyOptions(options *ImageProviderOptions) *storage.ImageCopyOptions {
	if options == nil {
		return &storage.ImageCopyOptions{}
	}
	
	copyOptions := &storage.ImageCopyOptions{
		SourceCtx:      options.SystemContext,
		DestinationCtx: options.SystemContext,
	}
	
	// Convert auth config if provided
	if options.Auth != nil {
		copyOptions.SourceCtx = &imageTypes.SystemContext{}
		if options.SystemContext != nil {
			*copyOptions.SourceCtx = *options.SystemContext // shallow copy
		}
		
		if options.Auth.Username != "" || options.Auth.Password != "" {
			copyOptions.SourceCtx.DockerAuthConfig = &imageTypes.DockerAuthConfig{
				Username: options.Auth.Username,
				Password: options.Auth.Password,
			}
		}
	}
	
	// Set up progress callback if provided
	if options.ProgressCallback != nil {
		// Create a channel for progress and start a goroutine to forward it
		progress := make(chan imageTypes.ProgressProperties)
		copyOptions.Progress = progress
		
		go func() {
			for p := range progress {
				options.ProgressCallback(p)
			}
		}()
	}
	
	return copyOptions
}

// convertImageResultToResult converts storage.ImageResult to ImageProviderResult
func convertImageResultToResult(imageResult *storage.ImageResult) *ImageProviderResult {
	if imageResult == nil {
		return nil
	}
	
	result := &ImageProviderResult{
		Size:   imageResult.Size,
		Digest: imageResult.Digest.String(),
		Labels: imageResult.Labels,
		Metadata: map[string]string{
			"provider": "default",
		},
	}
	
	if imageResult.SomeNameOfThisImage != nil {
		result.ImageRef = *imageResult.SomeNameOfThisImage
	}
	
	return result
}