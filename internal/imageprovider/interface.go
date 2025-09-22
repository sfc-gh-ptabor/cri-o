package imageprovider

import (
	"context"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/containers/image/v5/types"
	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ImageProvider defines the interface for pluggable image providers
// Each provider handles a specific type of image reference scheme or runtime handler
type ImageProvider interface {
	// Name returns the unique name/identifier of this provider
	Name() string

	// CanHandle determines if this provider can handle the given image spec
	// It should check the image reference scheme, runtime handler, or annotations
	CanHandle(imageSpec *criv1.ImageSpec) bool

	// PullImage pulls/prepares an image and returns a reference that can be used
	// by the container runtime. The returned reference should be in a format
	// that the storage layer can understand.
	PullImage(ctx context.Context, imageSpec *criv1.ImageSpec, options *ImageProviderOptions) (*ImageProviderResult, error)

	// ImageStatus returns the status of an image handled by this provider
	ImageStatus(ctx context.Context, imageSpec *criv1.ImageSpec) (*ImageProviderResult, error)

	// RemoveImage removes an image handled by this provider
	RemoveImage(ctx context.Context, imageSpec *criv1.ImageSpec) error

	// ListImages returns all images managed by this provider
	ListImages(ctx context.Context) ([]*ImageProviderResult, error)
}

// ImageProviderOptions contains options for image operations
type ImageProviderOptions struct {
	// Auth configuration for pulling images
	Auth *criv1.AuthConfig

	// PodSandboxConfig for context (namespace, labels, etc.)
	PodSandboxConfig *criv1.PodSandboxConfig

	// SystemContext from the image service
	SystemContext *types.SystemContext

	// ProgressCallback for reporting pull progress
	ProgressCallback func(types.ProgressProperties)
}

// ImageProviderResult represents the result of an image operation
type ImageProviderResult struct {
	// ImageRef is the canonical reference to the prepared image
	// This should be in a format that CRI-O's storage layer can use
	ImageRef storage.RegistryImageReference

	// Size of the image in bytes (optional)
	Size *uint64

	// Digest of the image (optional)
	Digest string

	// Labels from the image config (optional)
	Labels map[string]string

	// Additional metadata specific to the provider
	Metadata map[string]string
}

// Registry manages multiple image providers
type Registry struct {
	providers []ImageProvider
	// Default provider for standard container images
	defaultProvider ImageProvider
}

// NewRegistry creates a new provider registry
func NewRegistry(defaultProvider ImageProvider) *Registry {
	return &Registry{
		providers:       make([]ImageProvider, 0),
		defaultProvider: defaultProvider,
	}
}

// Register adds a new image provider to the registry
func (r *Registry) Register(provider ImageProvider) {
	r.providers = append(r.providers, provider)
}

// FindProvider finds the appropriate provider for the given image spec
func (r *Registry) FindProvider(imageSpec *criv1.ImageSpec) ImageProvider {
	// Check registered providers first
	for _, provider := range r.providers {
		if provider.CanHandle(imageSpec) {
			return provider
		}
	}
	
	// Fall back to default provider
	return r.defaultProvider
}

// GetProviders returns all registered providers
func (r *Registry) GetProviders() []ImageProvider {
	result := make([]ImageProvider, len(r.providers))
	copy(result, r.providers)
	return result
}