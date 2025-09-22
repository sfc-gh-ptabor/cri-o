# Pluggable Image Providers for CRI-O

This document describes the implementation of pluggable image providers in CRI-O, enabling support for different image types beyond standard container images.

## Overview

The pluggable image provider system allows CRI-O to handle different image schemes and sources through a provider-based architecture. This enables support for:

- Root filesystem directories (`root-fs://` scheme)
- Future custom image sources
- Extensible image handling without modifying core CRI-O code

## Architecture

### Components

1. **ImageProvider Interface** (`internal/imageprovider/interface.go`)
   - Defines the contract for image providers
   - Handles image operations: pull, status, remove, list

2. **RootFS Provider** (`internal/imageprovider/rootfs.go`)
   - Implements support for local filesystem directories
   - Handles `root-fs://` scheme URLs
   - Provides path validation and security controls

3. **Default Provider** (`internal/imageprovider/default.go`)
   - Wraps existing CRI-O image functionality
   - Handles standard container images (docker://, etc.)

4. **Service** (`internal/imageprovider/service.go`)
   - Manages provider registry
   - Routes requests to appropriate providers

### Configuration

Image providers are configured in the CRI-O configuration file under the `[crio.image.image_providers]` section:

```toml
[crio.image]
# Enable pluggable image providers
[crio.image.image_providers]
enable_pluggable_providers = true

# Root filesystem provider configuration
[crio.image.image_providers.rootfs]
enable = true
allowed_paths = [
    "/opt/containers",
    "/var/lib/crio-rootfs-images"
]
```

## Usage

### Root Filesystem Images

With the root filesystem provider enabled, you can reference local directories as container images:

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: my-app
    image: "root-fs:///opt/my-containers/my-app"
```

The directory at `/opt/my-containers/my-app` will be used directly as the container's root filesystem.

### Security Considerations

- **Path Restrictions**: The `allowed_paths` configuration restricts which directories can be used
- **Validation**: The provider validates that paths exist and are directories
- **Absolute Paths**: Only absolute paths are supported for security

## Implementation Details

### Image Provider Interface

```go
type ImageProvider interface {
    Name() string
    CanHandle(imageSpec *criv1.ImageSpec) bool
    PullImage(ctx context.Context, imageSpec *criv1.ImageSpec, options *ImageProviderOptions) (*ImageProviderResult, error)
    ImageStatus(ctx context.Context, imageSpec *criv1.ImageSpec) (*ImageProviderResult, error)
    RemoveImage(ctx context.Context, imageSpec *criv1.ImageSpec) error
    ListImages(ctx context.Context) ([]*ImageProviderResult, error)
}
```

### Provider Selection

The system selects providers based on the `CanHandle()` method. For root filesystem images:

```go
func (p *RootFSProvider) CanHandle(imageSpec *criv1.ImageSpec) bool {
    return strings.HasPrefix(imageSpec.GetImage(), "root-fs://")
}
```

### Integration Points

The pluggable system integrates with existing CRI-O image operations:

1. **PullImage** - Modified to use provider service when enabled
2. **ImageStatus** - Routes to appropriate provider
3. **RemoveImage** - Delegates to provider for handling
4. **Server Initialization** - Registers and configures providers

## Testing

Example test setup:

```bash
# Create test directory
mkdir -p /tmp/test-rootfs
echo "Hello from rootfs" > /tmp/test-rootfs/hello.txt

# Configure CRI-O with allowed path
# [crio.image.image_providers.rootfs]
# allowed_paths = ["/tmp"]

# Use in pod spec
# image: "root-fs:///tmp/test-rootfs"
```

## Future Extensibility

The provider system is designed for easy extension:

1. **New Providers**: Implement the `ImageProvider` interface
2. **Registration**: Add provider registration in server initialization
3. **Configuration**: Add provider-specific config sections

Example new provider:

```go
type MyCustomProvider struct{}

func (p *MyCustomProvider) CanHandle(imageSpec *criv1.ImageSpec) bool {
    return strings.HasPrefix(imageSpec.GetImage(), "mycustom://")
}
```

## Backward Compatibility

- When pluggable providers are disabled, CRI-O uses existing image handling
- Legacy image references work unchanged
- No breaking changes to existing functionality
- Gradual migration path for adopting new providers

## Files Modified/Added

### New Files
- `internal/imageprovider/interface.go` - Core provider interface
- `internal/imageprovider/service.go` - Provider management
- `internal/imageprovider/rootfs.go` - Root filesystem provider
- `internal/imageprovider/default.go` - Wrapper for existing functionality
- `internal/imageprovider/util.go` - Utility functions
- `server/image_provider_integration.go` - Integration with server

### Modified Files
- `pkg/config/config.go` - Added configuration structures
- `pkg/config/template.go` - Added configuration templates
- `server/server.go` - Added provider service initialization
- `server/image_pull.go` - Added provider-aware pull logic
- `server/image_status.go` - Added provider routing for status
- `server/image_remove.go` - Added provider routing for removal

This implementation provides a clean, extensible foundation for supporting diverse image sources in CRI-O while maintaining backward compatibility and security.