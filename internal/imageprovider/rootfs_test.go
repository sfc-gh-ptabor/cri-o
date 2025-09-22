package imageprovider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestRootFSProvider_CanHandle(t *testing.T) {
	provider := NewRootFSProvider([]string{})

	tests := []struct {
		name     string
		imageSpec *criv1.ImageSpec
		expected bool
	}{
		{
			name: "root-fs scheme should be handled",
			imageSpec: &criv1.ImageSpec{
				Image: "root-fs:///opt/containers/my-image",
			},
			expected: true,
		},
		{
			name: "docker scheme should not be handled",
			imageSpec: &criv1.ImageSpec{
				Image: "docker.io/library/nginx:latest",
			},
			expected: false,
		},
		{
			name: "empty image spec should not be handled",
			imageSpec: nil,
			expected: false,
		},
		{
			name: "empty image should not be handled",
			imageSpec: &criv1.ImageSpec{
				Image: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.CanHandle(tt.imageSpec)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestRootFSProvider_PullImage(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "rootfs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create some test content in the directory
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create provider with allowed paths
	provider := NewRootFSProvider([]string{tempDir})

	ctx := context.Background()
	imageSpec := &criv1.ImageSpec{
		Image: "root-fs://" + tempDir,
	}

	result, err := provider.PullImage(ctx, imageSpec, nil)
	if err != nil {
		t.Fatalf("PullImage failed: %v", err)
	}

	if result == nil {
		t.Fatal("PullImage returned nil result")
	}

	// Check that the result contains expected metadata
	if result.Metadata["rootfs.path"] != tempDir {
		t.Errorf("Expected rootfs.path to be %s, got %s", tempDir, result.Metadata["rootfs.path"])
	}

	if result.Metadata["rootfs.original_ref"] != imageSpec.Image {
		t.Errorf("Expected original_ref to be %s, got %s", imageSpec.Image, result.Metadata["rootfs.original_ref"])
	}
}

func TestRootFSProvider_PullImage_InvalidPath(t *testing.T) {
	provider := NewRootFSProvider([]string{})

	ctx := context.Background()
	imageSpec := &criv1.ImageSpec{
		Image: "root-fs:///nonexistent/path",
	}

	_, err := provider.PullImage(ctx, imageSpec, nil)
	if err == nil {
		t.Fatal("Expected PullImage to fail for nonexistent path")
	}
}

func TestRootFSProvider_PullImage_RestrictedPath(t *testing.T) {
	// Create a temporary directory for testing
	allowedDir, err := os.MkdirTemp("", "rootfs-allowed-*")
	if err != nil {
		t.Fatalf("Failed to create allowed temp dir: %v", err)
	}
	defer os.RemoveAll(allowedDir)

	restrictedDir, err := os.MkdirTemp("", "rootfs-restricted-*")
	if err != nil {
		t.Fatalf("Failed to create restricted temp dir: %v", err)
	}
	defer os.RemoveAll(restrictedDir)

	// Create provider with only one allowed path
	provider := NewRootFSProvider([]string{allowedDir})

	ctx := context.Background()

	// This should succeed
	allowedImageSpec := &criv1.ImageSpec{
		Image: "root-fs://" + allowedDir,
	}
	_, err = provider.PullImage(ctx, allowedImageSpec, nil)
	if err != nil {
		t.Fatalf("Expected allowed path to succeed: %v", err)
	}

	// This should fail
	restrictedImageSpec := &criv1.ImageSpec{
		Image: "root-fs://" + restrictedDir,
	}
	_, err = provider.PullImage(ctx, restrictedImageSpec, nil)
	if err == nil {
		t.Fatal("Expected restricted path to fail")
	}
}