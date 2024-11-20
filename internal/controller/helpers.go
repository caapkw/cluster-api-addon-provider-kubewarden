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

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	deployToAll = true

	kubewardenNamespace            = "kubewarden"
	kubewardenControllerRepository = "https://github.com/kubewarden/kubewarden-controller"
	githubReleasesPath             = "releases/download"
	kubewardenVersion              = "v1.18.0" // app version

	kubewardenHelmChartURL                = "https://charts.kubewarden.io/"
	kubewardenHelmReleaseName             = "caapkw"
	kubewardenHelmDefaultPolicyServerName = "default"

	defaultRequeueDuration = 1 * time.Minute

	KubewardenInstalledAnnotation = "caapkw.kubewarden.io/installed"
)

// applyManifest applies a single YAML manifest to the cluster
func applyManifest(ctx context.Context, k8sClient client.Client, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Error closing file: %v\n", err)
		}
	}()

	decoder := yaml.NewYAMLOrJSONDecoder(file, 1024)
	for {
		// use unknown to be able to decode any k8s object
		unk := &runtime.Unknown{}
		err := decoder.Decode(unk)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to decode manifest: %w", err)
		}

		// decode into a runtime.Object
		runtimeObject, kind, err := scheme.Codecs.UniversalDeserializer().Decode(unk.Raw, nil, nil)
		if kind == nil {
			// this is an invalid object, skip it
			break
		}
		if err != nil {
			return fmt.Errorf("failed to decode runtime object: %w", err)
		}
		obj, ok := runtimeObject.(client.Object)
		if !ok {
			return fmt.Errorf("failed to cast runtime object to client.Object")
		}
		// only create objects if they don't exist in the cluster already
		if err := k8sClient.Create(ctx, obj); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to apply resource: %w", err)
			}
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
	defer func() {
		if err := response.Body.Close(); err != nil {
			fmt.Printf("Error closing response body: %v\n", err)
		}
	}()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download file: HTTP %d", response.StatusCode)
	}

	tempFile, err := os.CreateTemp("", "kubewarden-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			fmt.Printf("Error closing temp file: %v\n", err)
		}
	}()

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
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Error closing file: %v\n", err)
		}
	}()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() {
		if err := gzipReader.Close(); err != nil {
			fmt.Printf("Error closing gzip reader: %v\n", err)
		}
	}()

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
		defer func() {
			if err := outFile.Close(); err != nil {
				fmt.Printf("Error closing file: %v\n", err)
			}
		}()

		if _, err := io.Copy(outFile, tarReader); err != nil {
			return "", fmt.Errorf("failed to write file %s: %w", targetPath, err)
		}
	}

	return extractDir, nil
}

type TemplateConfig struct {
	ReleaseName    string
	Chart          *chart.Chart
	Namespace      string
	IncludeCRDs    bool
	Values         map[string]interface{}
	UseReleaseName bool
}

func helmTemplate(config TemplateConfig) (string, error) {
	client := action.NewInstall(&action.Configuration{})

	client.ClientOnly = true
	client.DryRun = true
	client.ReleaseName = config.ReleaseName
	client.IncludeCRDs = config.IncludeCRDs
	client.Namespace = config.Namespace
	client.DisableHooks = true

	// Render chart.
	rel, err := client.Run(config.Chart, config.Values)
	if err != nil {
		return "", fmt.Errorf("could not render helm chart correctly: %w", err)
	}

	return rel.Manifest, nil
}

// HasAnnotation returns true if the object has the specified annotation.
func HasAnnotation(o metav1.Object, annotation string) bool {
	annotations := o.GetAnnotations()
	if annotations == nil {
		return false
	}

	_, ok := annotations[annotation]

	return ok
}
