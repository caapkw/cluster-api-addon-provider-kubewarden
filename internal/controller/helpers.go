package controller

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	deployToAll = true

	kubewardenNamespace            = "kubewarden"
	kubewardenControllerRepository = "https://github.com/kubewarden/kubewarden-controller"
	githubReleasesPath             = "releases/download"
	kubewardenVersion              = "v1.18.0"

	defaultRequeueDuration = 1 * time.Minute
)

// applyManifest applies a single YAML manifest to the cluster
func applyManifest(ctx context.Context, k8sClient client.Client, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	decoder := yaml.NewYAMLOrJSONDecoder(file, 1024)
	for {
		obj := &apiextensionsv1.CustomResourceDefinition{}
		err := decoder.Decode(obj)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to decode manifest: %w", err)
		}

		if err := k8sClient.Create(ctx, obj); err != nil {
			return fmt.Errorf("failed to apply resource: %w", err)
		}
	}

	return nil
}

// downloadFile downloads a file from a URL and returns the local path
func downloadFile(url string) (string, error) {
	response, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download file: HTTP %d", response.StatusCode)
	}

	tempFile, err := os.CreateTemp("", "kubewarden-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	_, err = io.Copy(tempFile, response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return tempFile.Name(), nil
}

// extractTarGz extracts a tar.gz file to a temporary directory
func extractTarGz(tarGzPath string) (string, error) {
	file, err := os.Open(tarGzPath)
	if err != nil {
		return "", fmt.Errorf("failed to open tar.gz file: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	extractDir, err := os.MkdirTemp("", "kubewarden-crds-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tar header: %w", err)
		}

		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}

		// Write each file
		targetPath := filepath.Join(extractDir, header.Name)
		outFile, err := os.Create(targetPath)
		if err != nil {
			return "", fmt.Errorf("failed to create file %s: %w", targetPath, err)
		}
		defer outFile.Close()

		if _, err := io.Copy(outFile, tarReader); err != nil {
			return "", fmt.Errorf("failed to write file %s: %w", targetPath, err)
		}
	}

	return extractDir, nil
}
