/*
Copyright The CBI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cbipluginhelper

import (
	"fmt"

	"github.com/cyphar/filepath-securejoin"
	corev1 "k8s.io/api/core/v1"

	crd "github.com/containerbuilding/cbi/pkg/apis/cbi/v1alpha1"
	pluginapi "github.com/containerbuilding/cbi/pkg/plugin/api"
)

type Helper struct {
	Image   string
	HomeDir string
}

// ContextInjector injects build contexts using `cbipluginhelper` image.
type ContextInjector struct {
	Helper
	TargetPodSpec      *corev1.PodSpec
	TargetContainerIdx int
}

// Inject injects a context to podSpec and returns the context path
func (ci *ContextInjector) Inject(bjContext crd.Context) (string, error) {
	switch k := bjContext.Kind; k {
	case crd.ContextKindConfigMap:
		return ci.injectConfigMap(bjContext.ConfigMapRef)
	case crd.ContextKindGit:
		return ci.injectGit(bjContext.Git)
	default:
		return "", fmt.Errorf("unsupported Spec.Context: %v", k)
	}
}

// injectConfigMap injects a config map to podSpec and returns the context path
func (ci *ContextInjector) injectConfigMap(configMapRef corev1.LocalObjectReference) (string, error) {
	const (
		// cmVol is a configmap volume (with symlinks)
		cmVolName      = "cbi-cmcontext-tmp"
		cmVolMountPath = "/cbi-cmcontext-tmp"
		// vol is an emptyDir volume (without symlinks)
		volName           = "cbi-cmcontext"
		volMountPath      = "/cbi-cmcontext"
		volContextSubpath = "context"
		// initContainer is used for converting cmVol to vol so as to eliminate symlinks
		initContainerName = "cbi-cmcontext-init"
	)
	idx := ci.TargetContainerIdx
	contextPath, _ := securejoin.SecureJoin(volMountPath, volContextSubpath)
	cmVol := corev1.Volume{
		Name: cmVolName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: configMapRef,
			},
		},
	}
	vol := corev1.Volume{
		Name: volName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	ci.TargetPodSpec.Volumes = append(ci.TargetPodSpec.Volumes, cmVol, vol)
	ci.TargetPodSpec.Containers[idx].VolumeMounts = append(ci.TargetPodSpec.Containers[idx].VolumeMounts,
		corev1.VolumeMount{
			Name:      volName,
			MountPath: volMountPath,
		},
	)
	initContainer := corev1.Container{
		Name:    initContainerName,
		Image:   ci.Helper.Image,
		Command: []string{"cp", "-rL", cmVolMountPath, contextPath},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      volName,
				MountPath: volMountPath,
			},
			{
				Name:      cmVolName,
				MountPath: cmVolMountPath,
			},
		},
	}
	ci.TargetPodSpec.InitContainers = append(ci.TargetPodSpec.InitContainers, initContainer)
	return contextPath, nil
}

// injectGit injects a git repo to podSpec and returns the context path
func (ci *ContextInjector) injectGit(spec crd.Git) (string, error) {
	const (
		// vol is an emptyDir volume
		volName           = "cbi-gitcontext"
		volMountPath      = "/cbi-gitcontext"
		volContextSubpath = "context"
		// initContainer is used for converting cmVol to vol so as to eliminate symlinks
		initContainerName = "cbi-gitcontext-init"
	)
	idx := ci.TargetContainerIdx

	ci.TargetPodSpec.Volumes = append(ci.TargetPodSpec.Volumes, corev1.Volume{
		Name: volName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
	ci.TargetPodSpec.Containers[idx].VolumeMounts = append(ci.TargetPodSpec.Containers[idx].VolumeMounts,
		corev1.VolumeMount{
			Name:      volName,
			MountPath: volMountPath,
		},
	)

	contextPath, _ := securejoin.SecureJoin(volMountPath, volContextSubpath)
	initContainer := corev1.Container{
		Name:  initContainerName,
		Image: ci.Helper.Image,
		Args:  []string{"populate-git", spec.URL, contextPath, "--revision", spec.Revision},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      volName,
				MountPath: volMountPath,
			},
		},
	}
	if spec.SubPath != "" {
		var err error
		contextPath, err = securejoin.SecureJoin(contextPath, spec.SubPath)
		if err != nil {
			return "", err
		}
	}
	if secretName := spec.SSHSecretRef.Name; secretName != "" {
		const sshVolName = "cbi-gitsshsecret"
		sshVolMountPath, err := securejoin.SecureJoin(ci.Helper.HomeDir, ".ssh")
		if err != nil {
			return "", err
		}
		defaultMode := int32(0400)
		sshVol := corev1.Volume{
			Name: sshVolName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  spec.SSHSecretRef.Name,
					DefaultMode: &defaultMode,
				},
			},
		}
		ci.TargetPodSpec.Volumes = append(ci.TargetPodSpec.Volumes, sshVol)
		initContainer.VolumeMounts = append(initContainer.VolumeMounts, corev1.VolumeMount{
			Name:      sshVolName,
			MountPath: sshVolMountPath,
		})
	}
	ci.TargetPodSpec.InitContainers = append(ci.TargetPodSpec.InitContainers, initContainer)
	return contextPath, nil
}

var Labels = map[string]string{
	pluginapi.LContextConfigMap: "",
	pluginapi.LContextGit:       "",
}
