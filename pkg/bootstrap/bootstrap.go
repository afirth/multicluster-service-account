/*
Copyright 2018 The Multicluster-Service-Account Authors.

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

package bootstrap

import (
	"context"
	"fmt"
	"time"

	"admiralty.io/multicluster-service-account/pkg/apis"
	"admiralty.io/multicluster-service-account/pkg/apis/multicluster/v1alpha1"
	"admiralty.io/multicluster-service-account/pkg/importer"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var namespace = "multicluster-service-account"
var deployName = "service-account-import-controller"
var clusterRoleName = "service-account-import-controller-remote"

func Bootstrap(srcCtx, srcKubeconfig, dstCtx, dstKubeconfig string) error {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = srcKubeconfig                              // if empty, env var or default path will be used instead
	overrides := &clientcmd.ConfigOverrides{CurrentContext: srcCtx} // if srcCtx is empty, kubeconfig's current context will be used instead
	srcCfgLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	srcCfg, err := srcCfgLoader.ClientConfig()
	if err != nil {
		return err
	}
	srcCfgRaw, err := srcCfgLoader.RawConfig()
	if err != nil {
		return err // TODO allow running bootstrap with in-cluster config
	}
	if srcCtx == "" {
		srcCtx = srcCfgRaw.CurrentContext
	}
	srcClusterName := srcCfgRaw.Contexts[srcCtx].Cluster
	// srcClient, err := client.New(srcCfg, client.Options{})
	// if err != nil {
	// 	return err
	// }
	srcClientset, err := kubernetes.NewForConfig(srcCfg)
	if err != nil {
		return err
	}

	rules = clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = dstKubeconfig                             // if empty, env var or default path will be used instead
	overrides = &clientcmd.ConfigOverrides{CurrentContext: dstCtx} // if dstCtx is empty, kubeconfig's current context will be used instead
	dstCfgLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	dstCfg, err := dstCfgLoader.ClientConfig()
	if err != nil {
		return err
	}
	dstCfgRaw, err := dstCfgLoader.RawConfig()
	if err != nil {
		return err // TODO allow running bootstrap with in-cluster config
	}
	if dstCtx == "" {
		dstCtx = dstCfgRaw.CurrentContext
	}
	dstClusterName := dstCfgRaw.Contexts[dstCtx].Cluster
	dstClientset, err := kubernetes.NewForConfig(dstCfg)
	if err != nil {
		return err
	}
	dstClient, err := client.New(dstCfg, client.Options{})
	if err != nil {
		return err
	}
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		return err
	}

	// The source cluster may not have have multicluster-service-account installed,
	// but it needs a service account that can read other service accounts and their token secrets.
	// We create that service account in the multicluster-service-account namespace,
	// and create that namespace if it doesn't exist.
	ns := &corev1.Namespace{}
	ns.Name = namespace
	_, err = srcClientset.CoreV1().Namespaces().Create(ns)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		fmt.Printf("namespace \"%s\" already exists in source cluster \"%s\"\n", namespace, srcCtx)
	} else {
		fmt.Printf("created namespace \"%s\" in source cluster \"%s\"\n", namespace, srcCtx)
	}

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"serviceaccounts", "secrets"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}
	_, err = srcClientset.RbacV1().ClusterRoles().Create(cr)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		fmt.Printf("cluster role \"%s\" already exists in source cluster \"%s\"\n", clusterRoleName, srcCtx)
	} else {
		fmt.Printf("created cluster role \"%s\" in source cluster \"%s\"\n", clusterRoleName, srcCtx)
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      dstClusterName,
		},
	}
	_, err = srcClientset.CoreV1().ServiceAccounts(namespace).Create(sa)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		fmt.Printf("service account \"%s\" already exists in namespace \"%s\" in source cluster \"%s\"\n", sa.Name, namespace, srcCtx)
	} else {
		fmt.Printf("created service account \"%s\" in namespace \"%s\" in source cluster \"%s\"\n", sa.Name, namespace, srcCtx)
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: dstClusterName,
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: clusterRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Namespace: sa.Namespace,
				Name:      sa.Name,
			},
		},
	}
	_, err = srcClientset.RbacV1().ClusterRoleBindings().Create(crb)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		fmt.Printf("cluster role binding \"%s\" already exists in source cluster \"%s\"\n", crb.Name, srcCtx)
	} else {
		fmt.Printf("created cluster role binding \"%s\" in source cluster \"%s\"\n", crb.Name, srcCtx)
	}

	var secretName string
	fmt.Printf("waiting until service account \"%s\" in namespace \"%s\" in source cluster \"%s\" has a token...\n", sa.Name, namespace, srcCtx)
	f := wait.ConditionFunc(func() (done bool, err error) {
		getSA, err := srcClientset.CoreV1().ServiceAccounts(sa.Namespace).Get(sa.Name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("cannot get service account \"%s\" in namespace \"%s\" in source cluster \"%s\": %v", sa.Name, namespace, srcCtx, err)
		}
		if len(getSA.Secrets) > 0 {
			secretName = getSA.Secrets[0].Name
			return true, nil
		}
		return false, nil
	})
	if err := wait.PollImmediate(time.Second, time.Minute, f); err != nil {
		return fmt.Errorf("timeout: %v", err)
	}

	saSecret, err := srcClientset.CoreV1().Secrets(sa.Namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("cannot get secret \"%s\" in namespace \"%s\" in source cluster \"%s\": %v", secretName, namespace, srcCtx, err)
	}
	if saSecret.Data == nil {
		return fmt.Errorf("secret \"%s\" in namespace \"%s\" in source cluster \"%s\" is empty", secretName, namespace, srcCtx)
	}

	sai := &v1alpha1.ServiceAccountImport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      srcClusterName,
		},
		Spec: v1alpha1.ServiceAccountImportSpec{
			ClusterName: srcClusterName,
			Namespace:   namespace,
			Name:        dstClusterName,
		},
	}
	if err := dstClient.Create(context.TODO(), sai); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		fmt.Printf("service account import \"%s\" already exists in namespace \"%s\" in target cluster \"%s\"\n", sai.Name, sai.Namespace, dstCtx)
		// in this case, the server doesn't return the state of sai, therefore it's missing a uid,
		// and the controller reference created below on the secret would be invalid if we do not get it
		if err := dstClient.Get(context.TODO(), types.NamespacedName{Name: sai.Name, Namespace: sai.Namespace}, sai); err != nil {
			return fmt.Errorf("cannot get service account import \"%s\" in namespace \"%s\" in target cluster \"%s\": %v", sai.Name, sai.Namespace, dstCtx, err)
		}
	} else {
		fmt.Printf("created service account import \"%s\" in namespace \"%s\" in target cluster \"%s\"\n", sai.Name, sai.Namespace, dstCtx)
	}

	saiSecret := importer.MakeServiceAccountImportSecret(sai, srcCfg.Host, saSecret, scheme.Scheme)
	saiSecret, err = dstClientset.CoreV1().Secrets(saiSecret.Namespace).Create(saiSecret)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("cannot create secret \"%s\" in namespace \"%s\" in target cluster \"%s\": %v", saiSecret.GenerateName, saiSecret.Namespace, dstCtx, err)
	}
	fmt.Printf("created secret \"%s\" in namespace \"%s\" in target cluster \"%s\"\n", saiSecret.GenerateName, saiSecret.Namespace, dstCtx)

	fmt.Printf("waiting until service account import \"%s\" in namespace \"%s\" in target cluster \"%s\" adopts token...\n", sai.Name, sai.Namespace, dstCtx)
	f = wait.ConditionFunc(func() (done bool, err error) {
		if err := dstClient.Get(context.TODO(), types.NamespacedName{Name: sai.Name, Namespace: sai.Namespace}, sai); err != nil {
			return false, fmt.Errorf("cannot get service account import \"%s\" in namespace \"%s\" in target cluster \"%s\": %v", sai.Name, sai.Namespace, dstCtx, err)
		}
		if len(sai.Status.Secrets) > 0 {
			return true, nil
		}
		return false, nil
	})
	if err := wait.PollImmediate(time.Second, time.Minute, f); err != nil {
		return fmt.Errorf("timeout: %v", err)
	}

	deploy, err := dstClientset.AppsV1().Deployments(namespace).Get(deployName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("cannot get deployment \"%s\" in namespace \"%s\" in target cluster \"%s\": %v", deployName, namespace, dstCtx, err)
	}
	imports, _ := deploy.Spec.Template.Annotations[importer.AnnotationKeyServiceAccountImportName]
	if imports != "" {
		imports += ","
	}
	imports += sai.Name
	deployCopy := deploy.DeepCopy()
	if deployCopy.Spec.Template.Annotations == nil {
		deployCopy.Spec.Template.Annotations = map[string]string{importer.AnnotationKeyServiceAccountImportName: imports}
	} else {
		deployCopy.Spec.Template.Annotations[importer.AnnotationKeyServiceAccountImportName] = imports
	}
	_, err = dstClientset.AppsV1().Deployments(namespace).Update(deployCopy)
	if err != nil {
		return fmt.Errorf("cannot annotate service account import controller in target cluster \"%s\": %v", dstCtx, err)
	}
	fmt.Printf("annotated service account import controller in target cluster \"%s\"\n", dstCtx)

	return nil
}
