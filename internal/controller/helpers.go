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
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
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

func createKubewardenNamespace(ctx context.Context, remoteClient client.Client) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: kubewardenNamespace,
		},
	}

	if err := remoteClient.Create(ctx, ns); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}

func renderHelmChart(ctx context.Context, name, version string, values map[string]interface{}) (string, error) {
	_, settings, err := createActionConfig(ctx, kubewardenNamespace)
	if err != nil {
		return "", err
	}

	var chartPathOptions action.ChartPathOptions = action.ChartPathOptions{
		RepoURL: kubewardenHelmChartURL,
		// this is chart version != app version & specific for each kubewarden chart
		// if empty, the latest version is used
		Version: version,
	}

	chart, err := getChart(chartPathOptions, name, settings)
	if err != nil {
		return "", err
	}

	rendered, err := helmTemplate(TemplateConfig{
		ReleaseName: kubewardenHelmReleaseName,
		Namespace:   kubewardenNamespace,
		Chart:       chart,
		Values:      values,
	})
	if err != nil {
		return "", err
	}

	renderedFile, err := os.CreateTemp("", "kubewarden-helm-rendered-*.yaml")
	if err != nil {
		return "", err
	}
	defer func() {
		if err := renderedFile.Close(); err != nil {
			fmt.Printf("Error closing temporary file: %v\n", err)
		}
	}()

	_, err = renderedFile.WriteString(rendered)
	if err != nil {
		return "", err
	}

	return renderedFile.Name(), nil
}

func createActionConfig(ctx context.Context, targetNamespace string) (*action.Configuration, *cli.EnvSettings, error) {
	log := log.FromContext(ctx)
	settings := cli.New()
	actionConfig := new(action.Configuration)

	err := actionConfig.Init(settings.RESTClientGetter(), targetNamespace, os.Getenv("HELM_DRIVER"), log.Info)

	return actionConfig, settings, err
}

func getChart(chartPathOption action.ChartPathOptions, chartName string, settings *cli.EnvSettings) (*chart.Chart, error) {
	chartPath, err := chartPathOption.LocateChart(chartName, settings)
	if err != nil {
		return nil, err
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, err
	}

	return chart, nil
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
