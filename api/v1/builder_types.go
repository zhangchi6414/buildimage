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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RunState string

const (
	NotRunning RunState = "Not Running Yet"
	Running    RunState = "Running"
	Successful          = "Successful"
	Failed              = "Failed"
	Unknown             = "Unknown"
)

//type Code struct {
//	Bucket   string `json:"bucket,omitempty" `
//	CodeName string `json:"codeName,omitempty"`
//	CodePath string `json:"codePath,omitempty"`
//}

type MinioOption struct {
	Endpoint   string `json:"endpoint,omitempty" `
	DisableSSL bool   `json:"disableSSL,omitempty"`
	//ForcePathStyle  string `json:"forcePathStyle,omitempty" `
	//Code            *[]Code `json:"code,omitempty"`
	Bucket          string `json:"bucket,omitempty" `
	CodeName        string `json:"codeName,omitempty"`
	CodePath        string `json:"codePath,omitempty"`
	AccessKeyID     string `json:"accessKeyID,omitempty" `
	SecretAccessKey string `json:"secretAccessKey,omitempty" `
	SessionToken    string `json:"sessionToken,omitempty" `
}
type HarborOption struct {
	Endpoint   string `json:"endpoint,omitempty"`
	DisableSSL bool   `json:"disableSSL,omitempty" `
	Username   string `json:"username,omitempty" `
	Password   string `json:"password,omitempty" `
}
type GitOption struct {
	Endpoint   string `json:"endpoint,omitempty"`
	DisableSSL bool   `json:"disableSSL,omitempty" `
	Username   string `json:"username,omitempty" `
	Password   string `json:"password,omitempty" `
}
type Buildconfig struct {
	BuildName      string        `json:"buildName,omitempty"`
	IsMinio        bool          `json:"IsMinio,omitempty"`
	Minio          *MinioOption  `json:"minio,omitempty"`
	IsSave         bool          `json:"IsSave,omitempty"`
	IsExport       bool          `json:"IsExport,omitempty"`
	HarborUrl      string        `json:"harborUrl,omitempty"`
	Harbor         *HarborOption `json:"harbor,omitempty"`
	NewImageName   string        `json:"newImageName,omitempty"`
	NewTag         string        `json:"newTag,omitempty"`
	IsGit          bool          `json:"isGit,omitempty"`
	Git            *GitOption    `json:"git,omitempty"`
	DockerfileName string        `json:"dockerfileName,omitempty"`
	BackLimit      int32         `json:"backLimit,omitempty"`
	SaveImageName  string        `json:"saveImageName,omitempty"`
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BuilderSpec defines the desired state of Builder
type BuilderSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Config *Buildconfig `json:"config,omitempty" `
}

// BuilderStatus defines the observed state of Builder
type BuilderStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	//RunCount represent the sum of s2irun of this builder
	RunCount int `json:"runCount"`
	//LastRunState return the state of the newest run of this builder
	LastRunState RunState `json:"lastRunState,omitempty"`
	//LastRunState return the name of the newest run of this builder
	LastRunName *string `json:"lastRunName,omitempty"`
	//LastRunStartTime return the startTime of the newest run of this builder
	LastRunStartTime *metav1.Time `json:"lastRunStartTime,omitempty"`
	StartTime        *metav1.Time `json:"startTime,omitempty" protobuf:"bytes,2,opt,name=startTime"`
	// Represents time when the job was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty" protobuf:"bytes,3,opt,name=completionTime"`
	// RunState  indicates whether this job is done or failed
	RunState RunState `json:"runState,omitempty"`
	//LogURL is uesd for external log handler to let user know where is log located in
	LogURL string `json:"logURL,omitempty"`
	//KubernetesJobName is the job name in k8s
	KubernetesJobName string `json:"kubernetesJobName,omitempty"`
}

// +genclient

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.runState"
// +kubebuilder:printcolumn:name="K8sJobName",type="string",JSONPath=".status.kubernetesJobName"
// +kubebuilder:printcolumn:name="StartTime",type="date",JSONPath=".status.startTime"
// +kubebuilder:printcolumn:name="CompletionTime",type="date",JSONPath=".status.completionTime"
// Builder is the Schema for the builders API
type Builder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuilderSpec   `json:"spec,omitempty"`
	Status BuilderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BuilderList contains a list of Builder
type BuilderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Builder `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Builder{}, &BuilderList{})
}
