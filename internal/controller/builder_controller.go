/*
Copyright 2023.

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

package controller

import (
	buildimagev1 "buildimage/buildimage/api/v1"
	"buildimage/buildimage/pkg"
	"context"
	"fmt"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

var log = logf.Log.WithName("build-image-controller")

// BuilderReconciler reconciles a Builder object
type BuilderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=buildimage.buildimage,resources=builders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=buildimage.buildimage,resources=builders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=buildimage.buildimage,resources=builders/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Builder object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *BuilderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//_ = log.FromContext(ctx)
	log.Info("开始执行构建任务：", "Name", req.Name)
	//判断资源是否被创建
	instance := &buildimagev1.Builder{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Println("not found resource~!")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}
	crs, err := createRBAC(ctx, r, instance)
	if err != nil {
		return crs, err
	}
	//查看job是否存在，不存在则创建
	Job := &v1.Job{}
	err = r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, Job)
	if err != nil {
		if errors.IsNotFound(err) {
			res, err := createJob(ctx, r, instance)
			return res, err
		} else {
			return ctrl.Result{}, err
		}
	}
	//检查job状态，更新build状态

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BuilderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&buildimagev1.Builder{}).
		Complete(r)
}
func slectFunc(instance *buildimagev1.Builder) (*v1.Job, error) {
	//判断获取代码文件的方式
	jobs := &pkg.Jobs{}
	if instance.Spec.Config.IsMinio {
		// TODO 创建从minio获取源码的方式
		jobs = &pkg.Jobs{
			Volume: []corev1.Volume{
				{
					Name: "dockerfileString",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: instance.Spec.Config.DockerfileName,
							},
							Items: []corev1.KeyToPath{
								{
									Key:  pkg.ConfigDataKey,
									Path: "Dockerfile",
								},
							},
						},
					},
				},
				{
					Name: "docker-sock",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/run/docker.sock",
						},
					},
				},
				{
					Name: "miniosecret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "minio",
						},
					},
				},
				{
					Name: "harborsecret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "harbor",
						},
					},
				},
			},
			VolumeMount: []corev1.VolumeMount{
				{
					Name:      "docker-sock",
					MountPath: "/var/run/docker.sock",
				},
				{
					Name:      "dockerfileString",
					MountPath: "/tmp/app/Dcokerfile",
				},
				{
					Name:      "miniosecret",
					MountPath: "/tmp/app/minio",
				},
				{
					Name:      "harborsecret",
					MountPath: "/tmp/app/harbor",
				},
			},
			Env: []corev1.EnvVar{
				{
					Name:  "MinioRUL",
					Value: instance.Spec.Config.Minio.Endpoint,
				},
				{
					Name:  "MinioForcePath",
					Value: instance.Spec.Config.Minio.ForcePathStyle,
				},
				{
					Name:  "MinioBucket",
					Value: instance.Spec.Config.Minio.Bucket,
				},
				{
					Name:  "Code",
					Value: instance.Spec.Config.Minio.CodeName,
				},

				{
					Name:  "NewImageName",
					Value: instance.Spec.Config.NewImageName,
				},
				{
					Name:  "NewTag",
					Value: instance.Spec.Config.NewTag,
				},
				{
					Name:  "JobType",
					Value: "minio",
				},
			},
		}
	}
	if instance.Spec.Config.IsSave {
		// TODO 创建通过Save方式上传镜像的方法
		jobs = &pkg.Jobs{
			Volume: []corev1.Volume{
				{
					Name: "docker-sock",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/run/docker.sock",
						},
					},
				},
				{
					Name: "miniosecret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "minio",
						},
					},
				},
				{
					Name: "harborsecret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "harbor",
						},
					},
				},
			},
			VolumeMount: []corev1.VolumeMount{
				{
					Name:      "docker-sock",
					MountPath: "/var/run/docker.sock",
				},
				{
					Name:      "miniosecret",
					MountPath: "/tmp/app/minio",
				},
				{
					Name:      "harborsecret",
					MountPath: "/tmp/app/harbor",
				},
			},
			Env: []corev1.EnvVar{
				{
					Name:  "MinioRUL",
					Value: instance.Spec.Config.Minio.Endpoint,
				},
				{
					Name:  "MinioForcePath",
					Value: instance.Spec.Config.Minio.ForcePathStyle,
				},
				{
					Name:  "MinioBucket",
					Value: instance.Spec.Config.Minio.Bucket,
				},
				{
					Name:  "Code",
					Value: instance.Spec.Config.Minio.CodeName,
				},
				{
					Name:  "SaveImageName",
					Value: instance.Spec.Config.SaveImageName,
				},
				{
					Name:  "NewImageName",
					Value: instance.Spec.Config.NewImageName,
				},
				{
					Name:  "NewTag",
					Value: instance.Spec.Config.NewTag,
				},
				{
					Name:  "JobType",
					Value: "save",
				},
			},
		}
	}
	if instance.Spec.Config.IsExport {
		// TODO 创建通过Export方式上传镜像的方法
		jobs = &pkg.Jobs{
			Volume: []corev1.Volume{
				{
					Name: "docker-sock",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/run/docker.sock",
						},
					},
				},
				{
					Name: "miniosecret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "minio",
						},
					},
				},
				{
					Name: "harborsecret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "harbor",
						},
					},
				},
			},
			VolumeMount: []corev1.VolumeMount{
				{
					Name:      "docker-sock",
					MountPath: "/var/run/docker.sock",
				},
				{
					Name:      "miniosecret",
					MountPath: "/tmp/app/minio",
				},
				{
					Name:      "harborsecret",
					MountPath: "/tmp/app/harbor",
				},
			},
			Env: []corev1.EnvVar{
				{
					Name:  "MinioRUL",
					Value: instance.Spec.Config.Minio.Endpoint,
				},
				{
					Name:  "MinioForcePath",
					Value: instance.Spec.Config.Minio.ForcePathStyle,
				},
				{
					Name:  "MinioBucket",
					Value: instance.Spec.Config.Minio.Bucket,
				},
				{
					Name:  "Code",
					Value: instance.Spec.Config.Minio.CodeName,
				},
				{
					Name:  "NewImageName",
					Value: instance.Spec.Config.NewImageName,
				},
				{
					Name:  "NewTag",
					Value: instance.Spec.Config.NewTag,
				},
				{
					Name:  "JobType",
					Value: "export",
				},
			},
		}
	}
	if instance.Spec.Config.IsGit {
		// TODO 创建从git获取源码的方式
		jobs = &pkg.Jobs{
			Volume: []corev1.Volume{
				{
					Name: "docker-sock",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/run/docker.sock",
						},
					},
				},
				{
					Name: "dockerfileString",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: instance.Spec.Config.DockerfileName,
							},
							Items: []corev1.KeyToPath{
								{
									Key:  pkg.ConfigDataKey,
									Path: "Dockerfile",
								},
							},
						},
					},
				},
				{
					Name: "harborsecret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "harbor",
						},
					},
				},
			},
			VolumeMount: []corev1.VolumeMount{
				{
					Name:      "docker-sock",
					MountPath: "/var/run/docker.sock",
				},
				{
					Name:      "harborsecret",
					MountPath: "/tmp/app/harbor",
				},
			},
			Env: []corev1.EnvVar{
				{
					Name:  "GirURL",
					Value: instance.Spec.Config.Git.Endpoint,
				},
				{
					Name:  "NewImageName",
					Value: instance.Spec.Config.NewImageName,
				},
				{
					Name:  "NewTag",
					Value: instance.Spec.Config.NewTag,
				},
				{
					Name:  "JobType",
					Value: "save",
				},
			},
		}

	}
	job, err := jobs.CreateJob(instance)
	if err != nil {
		return nil, err
	}
	return job, err
}

func createJob(ctx context.Context, r *BuilderReconciler, instance *buildimagev1.Builder) (ctrl.Result, error) {
	job, err := slectFunc(instance)
	if err != nil {
		panic(err)
	}
	fmt.Println(job)
	err = r.Create(ctx, job)
	if err != nil {
		//in some situation we cannot find job in cache, however it does exist in apiserver, in this case we just requeue
		if k8serror.IsAlreadyExists(err) {
			log.Info("Skip creating 'Already-Exists' job", "Job-Name", job.Name)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to create Job", "Namespace", job.Namespace, "Name", job.Name)
		return ctrl.Result{}, err
	} else {
		return ctrl.Result{}, nil
	}
}

func createRBAC(ctx context.Context, r *BuilderReconciler, instance *buildimagev1.Builder) (ctrl.Result, error) {
	//set Role
	cr := &v12.Role{}
	err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: pkg.RegularRoleName}, cr)
	if err != nil && k8serror.IsNotFound(err) {
		cr := r.NewRegularRole(pkg.RegularRoleName, instance.Namespace)
		log.Info("Creating Role", "name", cr.Name)
		err = r.Create(context.TODO(), cr)
		if err != nil {
			if k8serror.IsAlreadyExists(err) {
				log.Info("Skip creating 'Already-Exists' Role", "Role-Name", pkg.RegularRoleName)
				return reconcile.Result{RequeueAfter: time.Second * 5}, nil
			}
			log.Error(err, "Create Role failed", "name", cr.Name)
			return reconcile.Result{}, err
		}
		log.Info("Creating Role Success", "name", cr.Name)
	}

	//set service account
	sa := &corev1.ServiceAccount{}
	err = r.Get(context.TODO(), types.NamespacedName{Namespace: instance.Namespace, Name: pkg.RegularServiceAccount}, sa)
	if err != nil && k8serror.IsNotFound(err) {
		sa := r.NewServiceAccount(pkg.RegularServiceAccount, instance.Namespace)
		log.Info("Creating ServiceAccount", "Namespace", sa.Namespace, "name", sa.Name)
		err = r.Create(context.TODO(), sa)
		if err != nil {
			if k8serror.IsAlreadyExists(err) {
				log.Info("Skip creating 'Already-Exists' sa", "ServiceAccount-Name", pkg.RegularServiceAccount)
				return reconcile.Result{RequeueAfter: time.Second * 5}, nil
			}
			log.Error(err, "Create ServiceAccount failed", "Namespace", sa.Namespace, "name", sa.Name)
			return reconcile.Result{}, err
		}
		log.Info("Creating ServiceAccount Success", "Namespace", sa.Namespace, "name", sa.Name)
	}

	//set RoleBinding
	crb := &v12.RoleBinding{}
	err = r.Get(context.TODO(), types.NamespacedName{Namespace: instance.Namespace, Name: pkg.RegularRoleBinding}, crb)
	if err != nil && k8serror.IsNotFound(err) {
		crb := r.NewRoleBinding(pkg.RegularRoleBinding, pkg.RegularRoleName, sa.Name, sa.Namespace)
		log.Info("Creating RoleBinding", "Namespace", crb.Namespace, "name", crb.Name)
		err = r.Create(context.TODO(), crb)
		if err != nil {
			if k8serror.IsAlreadyExists(err) {
				log.Info("Skip creating 'Already-Exists' RoleBinding", "RoleBinding-Name", crb.Name)
				return reconcile.Result{RequeueAfter: time.Second * 5}, nil
			}
			log.Error(err, "Create RoleBinding failed", "Namespace", crb.Namespace, "name", crb.Name)
			return reconcile.Result{}, err
		}
		log.Info("Creating RoleBinding success", "Namespace", crb.Namespace, "name", crb.Name)
	}
	return reconcile.Result{}, nil
}
