package pkg

import (
	buildimagev1 "buildimage/buildimage/api/v1"
	v12 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Jobs struct {
	VolumeMount []v1.VolumeMount
	Volume      []v1.Volume
	Env         []v1.EnvVar
}

func (j *Jobs) CreateJob(instance *buildimagev1.Builder) (*v12.Job, error) {
	var job = &v12.Job{}
	jobName := instance.Name
	//jobName := instance.Name + fmt.Sprintf("-%s", utils.Randow()+"-job")
	//imageName := os.Getenv("BUILDIMAGENAME")
	imageName := "192.168.2.108:1180/fangzhou/buildrun:v0.0.24"
	//TODO 测试
	//if imageName == "" {
	//	return nil, fmt.Errorf("Failed to get s2i-image name, please set the env 'S2IIMAGENAME' ")
	//}
	job = &v12.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: instance.ObjectMeta.Namespace,
		},
		Spec: v12.JobSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"job-name": jobName},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: RegularServiceAccount,
					Containers: []v1.Container{
						{
							Name:            "buildimage",
							Image:           imageName,
							Command:         []string{"./builder"},
							ImagePullPolicy: v1.PullIfNotPresent,
							Resources: v1.ResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse("0.2"),
									v1.ResourceMemory: resource.MustParse("400M"),
								},
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse("0.4"),
									v1.ResourceMemory: resource.MustParse("800M"),
								},
							},
							Env:          j.Env,
							VolumeMounts: j.VolumeMount,
							SecurityContext: &v1.SecurityContext{
								Privileged: truePtr(),
							},
						},
					},
					RestartPolicy: v1.RestartPolicyNever,
					Volumes:       j.Volume,
				},
			},
			BackoffLimit: &instance.Spec.Config.BackLimit,
		},
	}
	return job, nil
}

func truePtr() *bool {
	t := true
	return &t
}
