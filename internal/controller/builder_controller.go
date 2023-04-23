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
	"buildimage/buildimage/utils"
	"context"
	newerror "errors"
	"fmt"
	"go.uber.org/zap"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
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
	//判断资源是否被创建
	zap.S().Info("Start Run", time.Now())
	instance := &buildimagev1.Builder{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	origin := instance.DeepCopy()
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !utils.ContainsString(instance.ObjectMeta.Finalizers, "builder", nil) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, "builder")
			if err := r.Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		if utils.ContainsString(instance.ObjectMeta.Finalizers, "builder", nil) {
			if err := r.DeleteBuildRuns(instance); err != nil {
				return reconcile.Result{}, err
			}
			instance.ObjectMeta.Finalizers = utils.RemoveString(instance.ObjectMeta.Finalizers, "builder", nil)
			if err := r.Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, err
			}
			zap.S().Info("Delete builder", "Namespace", instance.Namespace, "Name", instance.Name)
		}
		return reconcile.Result{}, nil
	}
	crs, err := createRBAC(ctx, r, instance)
	if err != nil {
		return crs, err
	}
	//查看job是否存在，不存在则创建
	job, err := slectFunc(instance)
	if err != nil {
		return ctrl.Result{}, err
	}
	//检查job状态，更新build状态
	found := &v1.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, found)
	if err != nil && k8serror.IsNotFound(err) {
		zap.S().Info("Start Creating Job", "Namespace", job.Namespace, "Name", job.Name)
		if err := controllerutil.SetControllerReference(instance, job, r.Scheme); err != nil {
			return reconcile.Result{}, err
		}
		err = r.Create(context.TODO(), job)
		if err != nil {
			//in some situation we cannot find job in cache, however it does exist in apiserver, in this case we just requeue
			if k8serror.IsAlreadyExists(err) {
				zap.S().Info("Skip creating 'Already-Exists' job", "Job-Name", job.Name)
				return reconcile.Result{RequeueAfter: time.Second * 5}, nil
			}
			zap.S().Error(err, "Failed to create Job", "Namespace", job.Namespace, "Name", job.Name)
			return reconcile.Result{}, err
		} else {
			zap.S().Info("Success Create job", job.Name, job.Namespace)
			return reconcile.Result{}, nil
		}
	}
	instance.Status.KubernetesJobName = found.Name
	instance.Status.StartTime = found.Status.StartTime
	if found.Status.Active == 1 {
		zap.S().Info("Job is running", "start time", found.Status.StartTime)
		instance.Status.RunState = buildimagev1.Running
		logURL, err := r.GetLogURL(found)
		if err != nil {
			return reconcile.Result{}, err
		}
		instance.Status.LogURL = logURL
	} else if found.Status.Failed == 1 {
		zap.S().Info("Job failed")
		instance.Status.RunState = buildimagev1.Failed
		instance.Status.CompletionTime = found.Status.CompletionTime
		logURL, err := r.GetLogURL(found)
		if err != nil {
			return reconcile.Result{}, err
		}
		instance.Status.LogURL = logURL
	} else if found.Status.Succeeded == 1 {
		zap.S().Info("Job completed", "time", found.Status.CompletionTime)
		instance.Status.RunState = buildimagev1.Successful
		instance.Status.CompletionTime = found.Status.CompletionTime
		logURL, err := r.GetLogURL(found)
		if err != nil {
			return reconcile.Result{}, err
		}
		instance.Status.LogURL = logURL
	} else {
		instance.Status.RunState = buildimagev1.Unknown
	}
	/*
		pods := &corev1.PodList{}
		err = r.List(context.TODO(), pods, client.InNamespace(Job.Namespace), client.MatchingLabels(map[string]string{
			"job-name": Job.Name,
		}))
		if err != nil && k8serror.IsNotFound(err) {
			zap.S().Info("Creating Job", "Namespace", Job.Namespace, "Name", Job.Name)
			if err := controllerutil.SetControllerReference(instance, Job, r.Scheme); err != nil {
				return reconcile.Result{}, err
			}
			err = r.Create(context.TODO(), Job)
			if err != nil {
				//in some situation we cannot find job in cache, however it does exist in apiserver, in this case we just requeue
				if k8serror.IsAlreadyExists(err) {
					zap.S().Info("Skip creating 'Already-Exists' job", "Job-Name", Job.Name)
					return reconcile.Result{RequeueAfter: time.Second * 5}, nil
				}
				log.Error(err, "Failed to create Job", "Namespace", Job.Namespace, "Name", Job.Name)
				return reconcile.Result{}, err
			} else {
				return reconcile.Result{}, nil
			}
		} else if err != nil {
			return reconcile.Result{}, err
		}
		instance.Status.KubernetesJobName = Job.Name
		instance.Status.StartTime = Job.Status.StartTime
		if Job.Status.Active == 1 {
			zap.S().Info("Job is running", "start time", Job.Status.StartTime)
			instance.Status.RunState = buildimagev1.Running
			logURL, err := r.GetLogURL(Job)
			if err != nil {
				return reconcile.Result{}, err
			}
			instance.Status.LogURL = logURL
		} else if Job.Status.Failed == 1 {
			zap.S().Info("Job failed")
			instance.Status.RunState = buildimagev1.Failed
			instance.Status.CompletionTime = Job.Status.CompletionTime
			logURL, err := r.GetLogURL(Job)
			if err != nil {
				return reconcile.Result{}, err
			}
			instance.Status.LogURL = logURL
		} else if Job.Status.Succeeded == 1 {
			zap.S().Info("Job completed", "time", Job.Status.CompletionTime)
			instance.Status.RunState = buildimagev1.Successful
			instance.Status.CompletionTime = Job.Status.CompletionTime
			logURL, err := r.GetLogURL(Job)
			if err != nil {
				return reconcile.Result{}, err
			}
			instance.Status.LogURL = logURL
		} else {
			instance.Status.RunState = buildimagev1.Unknown
		}
		fmt.Println(instance.Status)

	*/
	// set s2irun status
	pods := &corev1.PodList{}
	//err = r.List(context.TODO(), pods, client.InNamespace(found.Namespace), client.MatchingLabels(map[string]string{
	//	"job-name": found.Name,
	//}))
	err = r.List(context.TODO(), pods, client.InNamespace(found.Namespace), client.MatchingLabels(found.Spec.Template.Labels))
	if err != nil {
		log.Error(nil, "Error in get pod of job")
		return reconcile.Result{}, nil
	}
	if len(pods.Items) == 0 {
		return reconcile.Result{}, fmt.Errorf("cannot find any pod of the job %s", found.Name)
	}
	/*	foundPod := pods.Items[0]

			s2iBuildSource := &buildimagev1.{}
			if buildSource := foundPod.Annotations[AnnotationBuildSourceKey]; buildSource != "" {
				err = json.Unmarshal([]byte(buildSource), s2iBuildSource)
				if err != nil {
					return reconcile.Result{}, err
				}
			}
			instance.Status.S2iBuildSource = s2iBuildSource

			s2iBuildResult := &buildimagev1.S2iBuildResult{}
			if buildResult := foundPod.Annotations[AnnotationBuildResultKey]; buildResult != "" {
				err = json.Unmarshal([]byte(buildResult), s2iBuildResult)
				if err != nil {
					return reconcile.Result{}, err
				}
			}
			instance.Status.S2iBuildResult = s2iBuildResult

			//set default info in resource S2IRun's status.
			instance.Status.S2iBuildSource.BuilderImage = builder.Spec.Config.BuilderImage
			instance.Status.S2iBuildSource.SourceUrl = builder.Spec.Config.SourceURL
			instance.Status.S2iBuildSource.Description = builder.Spec.Config.Description
			instance.Status.S2iBuildResult.ImageName = builder.Spec.Config.ImageName
			if builder.Spec.Config.RevisionId == "" {
				instance.Status.S2iBuildSource.RevisionId = DefaultRevisionId
			} else {
				instance.Status.S2iBuildSource.RevisionId = builder.Spec.Config.RevisionId
			}

		// if job finished, scale workloads
		if instance.Status.RunState == buildimagev1.Successful || instance.Status.RunState == buildimagev1.Failed {
			err = r.ScaleWorkLoads(instance, builder)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	*/
	if !reflect.DeepEqual(instance.Status, origin.Status) {
		err = r.Status().Update(context.Background(), instance)
		if err != nil {
			log.Error(nil, "Failed to update s2irun status", "Namespace", instance.Namespace, "Name", instance.Name)
			return reconcile.Result{}, err
		}
	}
	if instance.Status.RunState == buildimagev1.Successful || instance.Status.RunState == buildimagev1.Failed || instance.Status.RunState == buildimagev1.Unknown {

		return ctrl.Result{}, nil
	}
	time.Sleep(10 * time.Second)
	return ctrl.Result{}, newerror.New("builder状态未达到预期")

}

// SetupWithManager sets up the controller with the Manager.
func (r *BuilderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&buildimagev1.Builder{}).
		Complete(r)
}

func (r *BuilderReconciler) DeleteBuildRuns(instance *buildimagev1.Builder) error {
	runList := new(buildimagev1.BuilderList)
	var errList []error
	err := r.Client.List(context.TODO(), runList, client.InNamespace(instance.Namespace))
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		// Error reading the object - requeue the request.
		return err
	}
	for _, item := range runList.Items {
		if item.Spec.Config.BuildName == instance.Name {
			err := r.Delete(context.TODO(), &item)
			if err != nil {
				errList = append(errList, err)
			}
		}
	}
	if len(errList) > 0 {
		zap.S().Error(errList)
		return newerror.New("删除builder出错！")
	}
	return nil

}
func (r *BuilderReconciler) GetLogURL(job *v1.Job) (string, error) {
	pods := &corev1.PodList{}

	err := r.List(context.TODO(), pods, client.InNamespace(job.Namespace), client.MatchingLabels(map[string]string{
		"job-name": job.Name,
	}))
	if err != nil {
		log.Error(nil, "Error in get pod of job")
		return "", nil
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("cannot find any pod of the job %s", job.Name)
	}

	//return loghandler.GetKubesphereLogger().GetURLOfPodLog(pods.Items[0].Namespace, pods.Items[0].Name)
	return "pod的URL", nil
}

func slectFunc(instance *buildimagev1.Builder) (*v1.Job, error) {

	//判断获取代码文件的方式
	jobs := &pkg.Jobs{}
	if instance.Spec.Config.IsMinio && instance.Spec.Config.Harbor != nil {
		// TODO 创建从minio获取源码的方式
		jobs = &pkg.Jobs{
			Volume: []corev1.Volume{
				{
					Name: "dockerfile",
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
				//	{
				//	Name: "miniosecret",
				//	VolumeSource: corev1.VolumeSource{
				//		Secret: &corev1.SecretVolumeSource{
				//			SecretName: "minio",
				//		},
				//	},
				//},
				//{
				//	Name: "harborsecret",
				//	VolumeSource: corev1.VolumeSource{
				//		Secret: &corev1.SecretVolumeSource{
				//			SecretName: "harbor",
				//		},
				//	},
				//},
			},
			VolumeMount: []corev1.VolumeMount{
				{
					Name:      "docker-sock",
					MountPath: "/var/run/docker.sock",
				},
				{
					Name:      "dockerfile",
					MountPath: "/config",
				},
			},
			Env: []corev1.EnvVar{
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
				{
					Name:  "HARBORK",
					Value: instance.Spec.Config.Harbor.Username,
				},
				{
					Name:  "HARBORV",
					Value: instance.Spec.Config.Harbor.Password,
				},
			},
		}
	} else if instance.Spec.Config.IsSave && instance.Spec.Config.Minio != nil && instance.Spec.Config.Harbor != nil {
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
			},
			VolumeMount: []corev1.VolumeMount{
				{
					Name:      "docker-sock",
					MountPath: "/var/run/docker.sock",
				},
			},
			Env: []corev1.EnvVar{
				{
					Name:  "MinioUrl",
					Value: instance.Spec.Config.Minio.Endpoint,
				},
				{
					Name:  "CodePath",
					Value: instance.Spec.Config.Minio.CodePath,
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
				{
					Name:  "MINIOK",
					Value: instance.Spec.Config.Minio.AccessKeyID,
				},
				{
					Name:  "MINIOV",
					Value: instance.Spec.Config.Minio.SecretAccessKey,
				},
				{
					Name:  "HARBORK",
					Value: instance.Spec.Config.Harbor.Username,
				},
				{
					Name:  "HARBORV",
					Value: instance.Spec.Config.Harbor.Password,
				},
			},
		}
	} else if instance.Spec.Config.IsExport && instance.Spec.Config.Minio != nil && instance.Spec.Config.Harbor != nil {
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
			},
			VolumeMount: []corev1.VolumeMount{
				{
					Name:      "docker-sock",
					MountPath: "/var/run/docker.sock",
				},
			},
			Env: []corev1.EnvVar{
				{
					Name:  "MinioUrl",
					Value: instance.Spec.Config.Minio.Endpoint,
				},
				{
					Name:  "CodePath",
					Value: instance.Spec.Config.Minio.CodePath,
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
				{
					Name:  "MINIOK",
					Value: instance.Spec.Config.Minio.AccessKeyID,
				},
				{
					Name:  "MINIOV",
					Value: instance.Spec.Config.Minio.SecretAccessKey,
				},
				{
					Name:  "HARBORK",
					Value: instance.Spec.Config.Harbor.Username,
				},
				{
					Name:  "HARBORV",
					Value: instance.Spec.Config.Harbor.Password,
				},
			},
		}
	} else if instance.Spec.Config.IsGit && instance.Spec.Config.Minio != nil && instance.Spec.Config.Harbor != nil {
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
					Name: "dockerfile",
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
			},
			VolumeMount: []corev1.VolumeMount{
				{
					Name:      "docker-sock",
					MountPath: "/var/run/docker.sock",
				},
				{
					Name:      "dockerfile",
					MountPath: "/config",
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
				{
					Name:  "MINIOK",
					Value: instance.Spec.Config.Minio.AccessKeyID,
				},
				{
					Name:  "MINIOV",
					Value: instance.Spec.Config.Minio.SecretAccessKey,
				},
				{
					Name:  "HARBORK",
					Value: instance.Spec.Config.Harbor.Username,
				},
				{
					Name:  "HARBORV",
					Value: instance.Spec.Config.Harbor.Password,
				},
			},
		}

	} else {
		return nil, newerror.New("结构不完整无法创建")
	}
	job, err := jobs.CreateJob(instance)
	if err != nil {
		return nil, err
	}
	return job, err
}

func createJob(ctx context.Context, r *BuilderReconciler, instance *buildimagev1.Builder) (ctrl.Result, error, *v1.Job) {

	job, err := slectFunc(instance)
	if err != nil {
		return ctrl.Result{}, err, job
	}
	err = r.Create(ctx, job)
	if err != nil {
		//in some situation we cannot find job in cache, however it does exist in apiserver, in this case we just requeue
		if k8serror.IsAlreadyExists(err) {
			zap.S().Info("Skip creating 'Already-Exists' job", "Job-Name", job.Name)
			return ctrl.Result{}, nil, job
		}
		log.Error(err, "Failed to create Job", "Namespace", job.Namespace, "Name", job.Name)
		return ctrl.Result{}, err, job
	} else {
		return ctrl.Result{}, nil, job
	}
}

func createRBAC(ctx context.Context, r *BuilderReconciler, instance *buildimagev1.Builder) (ctrl.Result, error) {
	//set Role
	cr := &v12.Role{}
	err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: pkg.RegularRoleName}, cr)
	if err != nil && k8serror.IsNotFound(err) {
		cr := r.NewRegularRole(pkg.RegularRoleName, instance.Namespace)
		zap.S().Info("Creating Role", "name", cr.Name)
		err = r.Create(context.TODO(), cr)
		if err != nil {
			if k8serror.IsAlreadyExists(err) {
				zap.S().Info("Skip creating 'Already-Exists' Role", "Role-Name", pkg.RegularRoleName)
				return reconcile.Result{RequeueAfter: time.Second * 5}, nil
			}
			log.Error(err, "Create Role failed", "name", cr.Name)
			return reconcile.Result{}, err
		}
		zap.S().Info("Creating Role Success", "name", cr.Name)
	}

	//set service account
	sa := &corev1.ServiceAccount{}
	err = r.Get(context.TODO(), types.NamespacedName{Namespace: instance.Namespace, Name: pkg.RegularServiceAccount}, sa)
	if err != nil && k8serror.IsNotFound(err) {
		sa := r.NewServiceAccount(pkg.RegularServiceAccount, instance.Namespace)
		zap.S().Info("Creating ServiceAccount", "Namespace", sa.Namespace, "name", sa.Name)
		err = r.Create(context.TODO(), sa)
		if err != nil {
			if k8serror.IsAlreadyExists(err) {
				zap.S().Info("Skip creating 'Already-Exists' sa", "ServiceAccount-Name", pkg.RegularServiceAccount)
				return reconcile.Result{RequeueAfter: time.Second * 5}, nil
			}
			log.Error(err, "Create ServiceAccount failed", "Namespace", sa.Namespace, "name", sa.Name)
			return reconcile.Result{}, err
		}
		zap.S().Info("Creating ServiceAccount Success", "Namespace", sa.Namespace, "name", sa.Name)
	}

	//set RoleBinding
	crb := &v12.RoleBinding{}
	err = r.Get(context.TODO(), types.NamespacedName{Namespace: instance.Namespace, Name: pkg.RegularRoleBinding}, crb)
	if err != nil && k8serror.IsNotFound(err) {
		crb := r.NewRoleBinding(pkg.RegularRoleBinding, pkg.RegularRoleName, sa.Name, sa.Namespace)
		zap.S().Info("Creating RoleBinding", "Namespace", crb.Namespace, "name", crb.Name)
		err = r.Create(context.TODO(), crb)
		if err != nil {
			if k8serror.IsAlreadyExists(err) {
				zap.S().Info("Skip creating 'Already-Exists' RoleBinding", "RoleBinding-Name", crb.Name)
				return reconcile.Result{RequeueAfter: time.Second * 5}, nil
			}
			log.Error(err, "Create RoleBinding failed", "Namespace", crb.Namespace, "name", crb.Name)
			return reconcile.Result{}, err
		}
		zap.S().Info("Creating RoleBinding success", "Namespace", crb.Namespace, "name", crb.Name)
	}
	return reconcile.Result{}, nil
}
