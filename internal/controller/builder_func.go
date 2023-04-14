package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *BuilderReconciler) NewRegularRole(roleName, namespace string) *v1.Role {
	cr := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
		},
		Rules: []v1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
		},
	}

	return cr
}

func (r *BuilderReconciler) NewServiceAccount(saName string, namespace string) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: namespace,
		},
	}
	return sa
}

func (r *BuilderReconciler) NewRoleBinding(name, roleName, saName, namespace string) *v1.RoleBinding {
	roleBinding := &v1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Subjects: []v1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: namespace,
			},
		},
		RoleRef: v1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
	}

	return roleBinding
}
