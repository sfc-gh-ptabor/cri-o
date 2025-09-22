package server

import (
	"context"

	"github.com/cri-o/cri-o/internal/imageprovider"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

// initializeImageProviderService sets up the pluggable image provider system
func (s *Server) initializeImageProviderService(ctx context.Context, config *libconfig.Config, imageService storage.ImageServer) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	
	// Check if pluggable providers are enabled
	if !config.ImageProviders.EnablePluggableProviders {
		log.Infof(ctx, "Pluggable image providers are disabled")
		s.imageProviderService = nil
		return nil
	}
	
	log.Infof(ctx, "Initializing pluggable image provider service")
	
	// Create the image provider service
	s.imageProviderService = imageprovider.NewService(imageService)
	
	// Register root filesystem provider if enabled
	if config.ImageProviders.RootFS.Enable {
		log.Infof(ctx, "Enabling root filesystem image provider with allowed paths: %v", config.ImageProviders.RootFS.AllowedPaths)
		rootfsProvider := imageprovider.NewRootFSProvider(config.ImageProviders.RootFS.AllowedPaths)
		s.imageProviderService.RegisterProvider(rootfsProvider)
	}
	
	providers := s.imageProviderService.GetProviders()
	log.Infof(ctx, "Image provider service initialized with providers: %v", providers)
	
	return nil
}