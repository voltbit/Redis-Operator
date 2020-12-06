// +build e2e_redis_op

package framework

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// Struct type with the resources generated by kustomize
type KustomizeConfig struct {
	Namespace          corev1.Namespace
	CRD                extv1.CustomResourceDefinition
	Role               rbacv1.Role
	ClusterRole        rbacv1.ClusterRole
	RoleBinding        rbacv1.RoleBinding
	ClusterRoleBinding rbacv1.ClusterRoleBinding
	Deployment         appsv1.Deployment
}

func NewKustomizeConfig() *KustomizeConfig {
	return &KustomizeConfig{
		Namespace:          corev1.Namespace{},
		CRD:                extv1.CustomResourceDefinition{},
		Role:               rbacv1.Role{},
		ClusterRole:        rbacv1.ClusterRole{},
		RoleBinding:        rbacv1.RoleBinding{},
		ClusterRoleBinding: rbacv1.ClusterRoleBinding{},
		Deployment:         appsv1.Deployment{},
	}
}

func (f *Framework) runKubectlApply(yamlData string) {
	var sout, serr bytes.Buffer
	cmd := exec.Command("kubectl apply", yamlData)
	cmd.Stdout = &sout
	cmd.Stderr = &serr
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[kubectl] kubectl apply failed: %v\n", err)
	}
	// TODO
	// need to wait for kubectl resource to be created to avoid flaky tests
	// must be replaced with kubectl wait
	// https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#wait
	time.Sleep(3 * time.Second)
}

func (f *Framework) runKustomizeCommand(command []string, path string) (string, string, error) {
	var sout, serr bytes.Buffer

	cmd := exec.Command("kustomize", command...)
	cmd.Dir = path
	cmd.Stdout = &sout
	cmd.Stderr = &serr
	err := cmd.Run()

	if err != nil {
		return sout.String(), serr.String(), errors.Wrapf(err, "kustomize incountered an error on command %s", command)
	} else if strings.TrimSpace(serr.String()) != "" {
		return sout.String(), serr.String(), errors.Errorf("kustomize encountered an error on command %s\n%s", command, serr.String())
	} else if strings.TrimSpace(sout.String()) == "" {
		return sout.String(), serr.String(), errors.Errorf("kustomize returned empty config for command %s", command)
	}

	return sout.String(), serr.String(), err
}

// BuildKustomizeConfig runs kustomize build on the specified path.
// It will also edit the name of the image.
func (f *Framework) BuildKustomizeConfig(configPath string, image string) (string, error) {
	fmt.Println("[E2E] Building kustomize config...")

	editCmd := []string{"edit", "image", "controller=" + image}
	sout, _, err := f.runKustomizeCommand(editCmd, "")
	if err != nil {
		return sout, err
	}

	buildCmd := []string{"build", configPath}
	sout, _, err = f.runKustomizeCommand(buildCmd, "")
	if err != nil {
		return sout, err
	}

	return sout, err
}

// get individual YAML documents from string
func getDocumentsFromString(res string) ([][]byte, error) {
	docs := [][]byte{}
	reader := k8syaml.NewYAMLReader(bufio.NewReader(strings.NewReader(res)))
	for {
		doc, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// ParseKustomizeConfig receives the multi-document YAML file generated by
// kustomize and generates the corresponding K8s resource objects and returns them
// as runtime objects and as YAML string map.
func (f *Framework) ParseKustomizeConfig(yamlConfig string) (*KustomizeConfig, map[string]string, error) {
	// TODO the parse and object generation relies on a particular format and
	// ordering of the YAML file generated by kustomize. The parser should be more flexible.
	// It should be done using unstrucutred objects and then parsing the kind + name like here:
	// https://github.com/kubernetes-sigs/controller-runtime/blob/v0.6.3/pkg/envtest/crd.go#L354
	fmt.Println("[E2E] Parsing kustomize config...")
	yamlMap := make(map[string]string)
	kustRes := NewKustomizeConfig()
	yamlRes, err := getDocumentsFromString(yamlConfig)
	if err != nil {
		fmt.Println("Could not read data from kustomize build")
	}

	if err := yaml.Unmarshal(yamlRes[0], &kustRes.Namespace); err != nil {
		fmt.Println("Could not unmarshal namespace resource")
		return nil, nil, err
	}
	yamlMap["namespace"] = string(yamlRes[0])

	if err := yaml.Unmarshal(yamlRes[1], &kustRes.CRD); err != nil {
		fmt.Println("Could not unmarshal CRD resource")
		return nil, nil, err
	}
	yamlMap["crd"] = string(yamlRes[1])

	if err := yaml.Unmarshal(yamlRes[2], &kustRes.Role); err != nil {
		fmt.Println("Could not unmarshal role resource")
		return nil, nil, err
	}
	yamlMap["role"] = string(yamlRes[2])

	if err := yaml.Unmarshal(yamlRes[3], &kustRes.ClusterRole); err != nil {
		fmt.Println("Could not unmarshal clusterrole resource")
		return nil, nil, err
	}
	yamlMap["clusterrole"] = string(yamlRes[3])

	if err := yaml.Unmarshal(yamlRes[4], &kustRes.RoleBinding); err != nil {
		fmt.Println("Could not unmarshal rolebinding resource")
		return nil, nil, err
	}
	yamlMap["rolebinding"] = string(yamlRes[4])

	if err := yaml.Unmarshal(yamlRes[5], &kustRes.ClusterRoleBinding); err != nil {
		fmt.Println("Could not unmarshal clusterrolebinding resource")
		return nil, nil, err
	}
	yamlMap["clusterrolebinding"] = string(yamlRes[5])

	if err := yaml.Unmarshal(yamlRes[6], &kustRes.Deployment); err != nil {
		fmt.Println("Could not unmarshal deployment resource")
		return nil, nil, err
	}
	yamlMap["deployment"] = string(yamlRes[6])

	f.KustomizeConfig = kustRes

	return kustRes, yamlMap, nil
}

func (f *Framework) BuildAndParseKustomizeConfig(configPath string, image string) (*KustomizeConfig, map[string]string, error) {
	yamlConfig, err := f.BuildKustomizeConfig(configPath, image)
	if err != nil {
		return nil, nil, err
	}

	kustRes, yamlMap, err := f.ParseKustomizeConfig(yamlConfig)
	if err != nil {
		return nil, nil, err
	}
	return kustRes, yamlMap, nil
}

func (f *Framework) isKubectlWarning(serr string) bool {
	errMsg := strings.Split(strings.TrimSpace(serr), " ")
	if len(errMsg) > 0 && errMsg[0] == "Warning:" {
		return true
	}
	return false
}

func (f *Framework) executeKubectlCommand(yamlRes string, args []string, timeout time.Duration, dryRun bool) (string, string, error) {
	var sout, serr bytes.Buffer

	if dryRun {
		args = append([]string{"--dry-run=client", "-o", "yaml"}, args...)
	}
	args = append([]string{"--request-timeout", timeout.String()}, args...)
	cmd := exec.Command("kubectl", args...)
	cmd.Stdin = strings.NewReader(yamlRes)
	cmd.Stdout = &sout
	cmd.Stderr = &serr

	err := cmd.Run()

	if err != nil {
		return sout.String(), serr.String(), errors.Wrap(err, "kubectl command returned an error")
	} else if strings.TrimSpace(serr.String()) != "" && !f.isKubectlWarning(serr.String()) {
		fmt.Printf("Kubectl output: %s\n", serr.String())
		return sout.String(), serr.String(), errors.Errorf("kubectl command returned an error: %s", serr.String())
	}
	return sout.String(), serr.String(), err
}

func (f *Framework) kubectlApply(yamlResource string, timeout time.Duration, dryRun bool) (string, string, error) {
	fmt.Println("[kubectl] Running apply...")
	applyCmd := []string{"apply", "-f", "-"}
	return f.executeKubectlCommand(yamlResource, applyCmd, timeout, dryRun)
}

func (f *Framework) kubectlDelete(yamlResource string, timeout time.Duration) (string, string, error) {
	fmt.Println("[kubectl] Running delete...")
	deleteCmd := []string{"delete", "-f", "-"}
	return f.executeKubectlCommand(yamlResource, deleteCmd, timeout, false)
}
