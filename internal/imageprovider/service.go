package imageprovider

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Service manages image providers and routes image operations to the appropriate provider
type Service struct {
	registry *Registry
}

// NewService creates a new image provider service
func NewService(imageService storage.ImageServer) *Service {
	// Create registry with default provider
	defaultProvider := NewDefaultProvider(imageService)
	registry := NewRegistry(defaultProvider)
	
	return &Service{
		registry: registry,
	}
}

// RegisterProvider registers a new image provider
func (s *Service) RegisterProvider(provider ImageProvider) {
	s.registry.Register(provider)
}

// PullImage pulls an image using the appropriate provider
func (s *Service) PullImage(ctx context.Context, imageSpec *criv1.ImageSpec, options *ImageProviderOptions) (*ImageProviderResult, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	
	provider := s.registry.FindProvider(imageSpec)
	
	log.Infof(ctx, "Using image provider %q for image %q", provider.Name(), imageSpec.GetImage())
	
	result, err := provider.PullImage(ctx, imageSpec, options)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image with provider %q: %w", provider.Name(), err)
	}
	
	return result, nil
}

// ImageStatus gets image status using the appropriate provider
func (s *Service) ImageStatus(ctx context.Context, imageSpec *criv1.ImageSpec) (*ImageProviderResult, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	
	provider := s.registry.FindProvider(imageSpec)
	
	result, err := provider.ImageStatus(ctx, imageSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to get image status with provider %q: %w", provider.Name(), err)
	}
	
	return result, nil
}

// RemoveImage removes an image using the appropriate provider
func (s *Service) RemoveImage(ctx context.Context, imageSpec *criv1.ImageSpec) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	
	provider := s.registry.FindProvider(imageSpec)
	
	log.Infof(ctx, "Removing image %q using provider %q", imageSpec.GetImage(), provider.Name())
	
	err := provider.RemoveImage(ctx, imageSpec)
	if err != nil {
		return fmt.Errorf("failed to remove image with provider %q: %w", provider.Name(), err)
	}
	
	return nil
}

// ListImages lists all images from all providers
func (s *Service) ListImages(ctx context.Context) ([]*ImageProviderResult, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	
	var allResults []*ImageProviderResult
	
	// Get images from default provider
	defaultResults, err := s.registry.defaultProvider.ListImages(ctx)
	if err != nil {
		log.Warnf(ctx, "Failed to list images from default provider: %v", err)
	} else {
		allResults = append(allResults, defaultResults...)
	}
	
	// Get images from all registered providers
	for _, provider := range s.registry.GetProviders() {
		results, err := provider.ListImages(ctx)
		if err != nil {
			log.Warnf(ctx, "Failed to list images from provider %q: %v", provider.Name(), err)
			continue
		}
		allResults = append(allResults, results...)
	}
	
	return allResults, nil
}

// GetProviders returns information about all registered providers
func (s *Service) GetProviders() []string {
	names := []string{s.registry.defaultProvider.Name()}
	for _, provider := range s.registry.GetProviders() {
		names = append(names, provider.Name())
	}
	return names
}