package awslib 

import (
  "fmt"
  "errors"
  "github.com/aws/aws-sdk-go/aws"
  "github.com/aws/aws-sdk-go/service/ecs"
  "github.com/aws/aws-sdk-go/service/ec2"
  "github.com/spf13/viper"
)

type Cluster struct {
  Arn *string
}


//
// CLUSTERS
//

func CreateCluster(clusterName string, svc *ecs.ECS) (*ecs.Cluster, error) {
  params := &ecs.CreateClusterInput{
    ClusterName: aws.String(clusterName),
  }
  resp, err := svc.CreateCluster(params)
  var cluster *ecs.Cluster
  if err == nil {
    cluster = resp.Cluster
  }
  return cluster, err
}

func DeleteCluster(clusterName string, svc *ecs.ECS) (*ecs.Cluster, error) {
  params := &ecs.DeleteClusterInput{
    Cluster: aws.String(clusterName),
  }
  resp, err := svc.DeleteCluster(params)
  var cluster *ecs.Cluster
  if err == nil {
    cluster = resp.Cluster
  }
  return cluster, err
}

func GetClusters(svc *ecs.ECS) ([]Cluster, error) {

  params := &ecs.ListClustersInput {
    MaxResults: aws.Int64(100),
  } // TODO: this only will get the first 100 ....
  output, err := svc.ListClusters(params)
  if err != nil{ return []Cluster{}, err }

  clusters := make([]Cluster, len(output.ClusterArns))
  for i, arn := range output.ClusterArns {
    clusters[i] = Cluster{Arn: arn}
  }

  return clusters, nil
}

func DescribeCluster(clusterName string, svc *ecs.ECS) ([]*ecs.Cluster, error) {
  
  params := &ecs.DescribeClustersInput {
    Clusters: []*string{aws.String(clusterName),},
  }

  resp, err := svc.DescribeClusters(params)
  return resp.Clusters, err
}

//
// CONTAINER INSTANCES
//

func GetContainerInstances(clusterName string, svc *ecs.ECS)([]*string, error) {
  params := &ecs.ListContainerInstancesInput {
    Cluster: aws.String(clusterName),
    MaxResults: aws.Int64(100),
  }
  resp, err := svc.ListContainerInstances(params)
  if err != nil { return []*string{}, err }

  return resp.ContainerInstanceArns, nil
}


type ContainerInstance struct {
  Instance  *ecs.ContainerInstance
  Failure *ecs.Failure
}
type ContainerInstanceMap map[string]*ContainerInstance

func GetAllContainerInstanceDescriptions(clusterName string, svc *ecs.ECS) (ContainerInstanceMap, error) {

  instanceArns, err := GetContainerInstances(clusterName, svc)
  if err != nil { return make(ContainerInstanceMap), err }

  if len(instanceArns) <= 0 {
    return make(ContainerInstanceMap), nil
  }

  params := &ecs.DescribeContainerInstancesInput {
    ContainerInstances: instanceArns,
    Cluster: aws.String(clusterName),
  }
  resp, err := svc.DescribeContainerInstances(params)
  return makeCIMapFromDescribeContainerInstancesOutput(resp), err
}

func GetContainerInstanceDescription(clusterName string, containerArn string, ecs_svc *ecs.ECS) (ContainerInstanceMap, error) {

  params := &ecs.DescribeContainerInstancesInput{
    ContainerInstances: []*string{aws.String(containerArn)},
    Cluster: aws.String(clusterName),
  }
  resp, err := ecs_svc.DescribeContainerInstances(params)
  return makeCIMapFromDescribeContainerInstancesOutput(resp), err
}

func makeCIMapFromDescribeContainerInstancesOutput(dcio *ecs.DescribeContainerInstancesOutput) (ContainerInstanceMap) {

  ciMap := make(ContainerInstanceMap)
  // Seperate out the conatiners instances ...
  for _, instance := range dcio.ContainerInstances {
    ci := new(ContainerInstance)
    ci.Instance = instance
    ciMap[*instance.ContainerInstanceArn] =  ci
  }
  // ... and failures.
  for _, failure := range dcio.Failures {
    ci := ciMap[*failure.Arn]
    if ci == nil {
      ci := new(ContainerInstance)
      ci.Failure = failure
      ciMap[*failure.Arn] = ci
    } else {
      ci.Failure = failure
    }
  }
  return ciMap
}

func (ciMap ContainerInstanceMap) GetEc2InstanceIds() ([]*string) {
  ids := []*string{}
  for _, ci := range ciMap {
    if ci.Instance != nil {
      ids = append(ids, ci.Instance.Ec2InstanceId)
    }
  }
  return ids
}

func TerminateContainerInstance(clusterName string, containerArn string, ecs_svc *ecs.ECS, ec2Svc *ec2.EC2) (resp *ec2.TerminateInstancesOutput, err error) {

  // Need to get the container instance description in order to get the ec2-instanceID.
  params := &ecs.DescribeContainerInstancesInput{
    ContainerInstances: []*string{aws.String(containerArn)},
    Cluster: aws.String(clusterName),
  }
  dci_resp, err := ecs_svc.DescribeContainerInstances(params)
  if err != nil {
    return nil, err
  }

  instanceId := getInstanceId(dci_resp.ContainerInstances, containerArn)
  if instanceId == nil {
    errMessage := fmt.Sprintf("TerminateContainerInstance: Can't find the Ec2 Instance ID for container arn: %s", containerArn)
    err = errors.New(errMessage)
    resp = nil
  } else {
   resp, err = TerminateInstance(instanceId, ec2Svc)
  }

  return resp, err
}

func getInstanceId(containerInstances []*ecs.ContainerInstance, containerArn string) (instanceId *string) {
  for _, instance := range containerInstances {
    if *instance.ContainerInstanceArn == containerArn {
      instanceId = instance.Ec2InstanceId
    }
  }
  return instanceId
}

//
// TASKS
//

// TODO: Add overides.
func ListTasksForCluster(clusterName string, ecs_svc *ecs.ECS) ([]*string, error) {

  params := &ecs.ListTasksInput{
    Cluster: aws.String(clusterName),
    MaxResults: aws.Int64(100),
  }
  resp, err := ecs_svc.ListTasks(params)
  return resp.TaskArns, err
}

type ContainerTask struct {
  Task *ecs.Task
  Failure *ecs.Failure
}
type ContainerTaskMap map[string]*ContainerTask

func GetAllTaskDescriptions(clusterName string, ecs_svc *ecs.ECS) (ContainerTaskMap, error) {
 
 taskArns, err := ListTasksForCluster(clusterName, ecs_svc)
 if err != nil { return make(ContainerTaskMap), err}

 // Describe task will fail with no arns.
 if len(taskArns) <= 0 {
  return make(ContainerTaskMap), nil
 }

  params := &ecs.DescribeTasksInput {
    Cluster: aws.String(clusterName),
    Tasks: taskArns,
  }

  resp, err := ecs_svc.DescribeTasks(params)
  return makeCTMapFromDescribeTasksOutput(resp), err
}

func makeCTMapFromDescribeTasksOutput(dto *ecs.DescribeTasksOutput) (ContainerTaskMap) {
  ctMap := make(ContainerTaskMap)
  for _, task := range dto.Tasks {
    ct := new(ContainerTask)
    ct.Task = task
    ctMap[*task.TaskArn] =  ct
  }
  for _, failure := range dto.Failures {
    ct := ctMap[*failure.Arn]
    if ct == nil {
      ct := new(ContainerTask)
      ct.Failure = failure
      ctMap[*failure.Arn] = ct
    } else {
      ct.Failure = failure
    }
  }
  return ctMap
}

func RunTask(clusterName string, taskDef string, ecs_svc *ecs.ECS) (*ecs.RunTaskOutput, error) {

  params := &ecs.RunTaskInput{
    TaskDefinition: aws.String(taskDef),
    Cluster: aws.String(clusterName),
    Count: aws.Int64(1),
  }
  resp, err := ecs_svc.RunTask(params)
  return resp, err
}

func OnTaskRunning(clusterName, taskDefArn string, ecs_svc *ecs.ECS, do func(error)) {
    go func() {
      params := &ecs.DescribeTasksInput{
        Cluster: aws.String(clusterName),
        Tasks: []*string{aws.String(taskDefArn)},
      }
      err := ecs_svc.WaitUntilTasksRunning(params)
      do(err)
    }()
}

func StopTask(clusterName string, taskArn string, ecs_svc *ecs.ECS) (*ecs.StopTaskOutput, error)  {
 params := &ecs.StopTaskInput{
    Task: aws.String(taskArn),
    Cluster: aws.String(clusterName),
  }
  resp, err := ecs_svc.StopTask(params)
  return resp, err
}

func OnTaskStopped(clusterName, taskArn string, ecs_svc *ecs.ECS, do func(error)) {
  go func() {
    params := &ecs.DescribeTasksInput{
      Cluster: aws.String(clusterName),
      Tasks: []*string{aws.String(taskArn)},
    }
    err := ecs_svc.WaitUntilTasksStopped(params)
    do(err)
  }()
}

func ListTaskDefinitions(ecs_svc *ecs.ECS) ([]*string, error) {
  params := &ecs.ListTaskDefinitionsInput{
    MaxResults: aws.Int64(100),
  }
  resp, err := ecs_svc.ListTaskDefinitions(params)
  return resp.TaskDefinitionArns, err
}

//
// Task Definitions
//

func GetTaskDefinition(taskDefinitionArn string, ecs_svc *ecs.ECS) (*ecs.TaskDefinition, error) {
  params := &ecs.DescribeTaskDefinitionInput {
    TaskDefinition: aws.String(taskDefinitionArn),
  }
  resp, err := ecs_svc.DescribeTaskDefinition(params)
  return resp.TaskDefinition, err
}

func RegisterTaskDefinition(configFileName string, ecs_svc *ecs.ECS) (*ecs.RegisterTaskDefinitionOutput, error) {
  config := viper.New()
  config.SetConfigName(configFileName)
  config.AddConfigPath(".")
  err := config.ReadInConfig()
  if err != nil {
    fmt.Printf("Couldn't read the config file.\n")
    return nil, err
  }

  var tdi ecs.RegisterTaskDefinitionInput
  err = config.Unmarshal(&tdi)
  if err != nil {
    fmt.Printf("Couldn't unmarshall the config file.\n")
    return nil, err
  }
  fmt.Printf("Registering Task definition: %+v\n", tdi)
  resp, err := ecs_svc.RegisterTaskDefinition(&tdi)
  if err == nil {
    fmt.Printf("Task Definnition registered: %+v\n", resp)
  } 
  return nil, err
}

func DefaultTaskDefinition() (ecs.RegisterTaskDefinitionInput) {
    var tdi = ecs.RegisterTaskDefinitionInput{
    Family: aws.String("Family"),
    // TaskRoleArn: This appears not to be in the golang definition.
    ContainerDefinitions: []*ecs.ContainerDefinition{
      &ecs.ContainerDefinition{

        // REQUIRED Basic paramaters
        //
        Name: aws.String("Task Definition Name"),

        Image: aws.String("IMAGE REFERENCE"),
        // Maximum memory in MB (recomended 300-500 if unsure.)
        // Conatiner is killed if you try to exceed this amount of memory.
        Memory: aws.Int64(500),        // DOCKER CMD

        // Other Basic Components
        //
        Command: []*string{aws.String("CMD"),},
        // DOCKER Entrypoint.
        EntryPoint: []*string{
          aws.String("ENTRYPOINT"),
        },
        DockerLabels: nil,

        // Environment
        //
        // The number of CPUY units to reserve for this container, there are 1024 for each EC2 core.
        Cpu: aws.Int64(0),
        // If marked true, then the failure of this coantiner will stop the task.
        Essential: aws.Bool(true),
        // Working directory to run binaries from.
        WorkingDirectory: nil,
        // Environment Variables
        Environment: nil,

        // Networking
        //
        // When true, this means networking is disabled within the container.  (defaulat: false).
        DisableNetworking: aws.Bool(false),
        PortMappings: []*ecs.PortMapping{
          {
            ContainerPort: aws.Int64(25565),
            HostPort: aws.Int64(25565),
            Protocol: aws.String("tcp"),
          },
        },
        // Hostname to use for your container.
        Hostname: nil,
        // DNS Servers presented to the container.
        DnsServers: nil,
        // DNS Search domains presented to the container.
        DnsSearchDomains: nil,
        // Enties to append to /etc/hosts.
        ExtraHosts: nil,

        // Storage
        //
        // If true then the container is given only readonly access to the root filesystem.
        ReadonlyRootFilesystem: aws.Bool(false),
        // this is like the --volumes option in the docker run command.
        MountPoints: nil,
        VolumesFrom: nil,

        // Logs
        LogConfiguration: nil,

        // Security
        //
        // Elevated privileges when container is run - like root.
        Privileged: aws.Bool(false),
        // run commands as this user.
        User: nil,
        // Labels for SELinux and AppArmour 
        DockerSecurityOptions: nil,

        // Resource Limits
        //
        // A list of ulimits to set in the container.
        // (eg. CORE, CPU, FSIZE, LOCKS, MLOCK, MSGQUEUE, NICE, NFILE, NPROC, RSS, RTPRIO, RTTIME, SIGPENDING, STACK)
        Ulimits: nil,
      },
    },
    Volumes: []*ecs.Volume{},
  }
  return tdi
}

func CompleteEmptyTaskDefinition() (ecs.RegisterTaskDefinitionInput) {
    var tdi = ecs.RegisterTaskDefinitionInput{
    Family: aws.String(""),
    // TaskRoleArn: This appears not to be in the golang definition.
    ContainerDefinitions: []*ecs.ContainerDefinition{
      &ecs.ContainerDefinition{

        // REQUIRED Basic paramaters
        //
        Name: aws.String(""),

        Image: aws.String(""),
        // Maximum memory in MB (recomended 300-500 if unsure.)
        // Conatiner is killed if you try to exceed this amount of memory.
        Memory: aws.Int64(0),        // DOCKER CMD

        // Other Basic Components
        //
        Command: []*string{aws.String(""),},
        // DOCKER Entrypoint.
        EntryPoint: []*string{
          aws.String(""),
        },
        DockerLabels: map[string]*string {
          "Key": aws.String("Value"),
        },

        // Environment
        //
        // The number of CPUY units to reserve for this container, there are 1024 for each EC2 core.
        Cpu: aws.Int64(0),
        // If marked true, then the failure of this coantiner will stop the task.
        Essential: aws.Bool(true),
        // Working directory to run binaries from.
        WorkingDirectory: aws.String(""),
        // Environment Variables
        Environment: []*ecs.KeyValuePair{
          {
            Name: aws.String(""),
            Value: aws.String(""),
          },
        },

        // Networking
        //
        // When true, this means networking is disabled within the container.  (defaulat: false).
        DisableNetworking: aws.Bool(false),
        PortMappings: []*ecs.PortMapping{
          {
            ContainerPort: aws.Int64(1),
            HostPort: aws.Int64(1),
            Protocol: aws.String("tcp"),
          },
        },
        // Hostname to use for your container.
        Hostname: aws.String(""),
        // DNS Servers presented to the container.
        DnsServers: []*string{
          aws.String(""),
        },
        // DNS Search domains presented to the container.
        DnsSearchDomains: []*string{
          aws.String(""),
        },
        // Enties to append to /etc/hosts.
        ExtraHosts: []*ecs.HostEntry{
          {
            Hostname: aws.String(""),
            IpAddress: aws.String(""),
          },
        },

        // Storage
        //
        // If true then the container is given only readonly access to the root filesystem.
        ReadonlyRootFilesystem: aws.Bool(false),
        // this is like the --volumes option in the docker run command.
        MountPoints: []*ecs.MountPoint{
          {
            ContainerPath: aws.String(""),
            ReadOnly: aws.Bool(false),
            SourceVolume: aws.String(""),
          },
        },
        VolumesFrom: []*ecs.VolumeFrom{
          {
            ReadOnly: aws.Bool(false),
            SourceContainer: aws.String(""),
          },
        },

        // Logs
        LogConfiguration: &ecs.LogConfiguration{
          LogDriver: aws.String("LogDriver"),
          Options: map[string]*string{
            "Key": aws.String(""),
          },
        },

        // Security
        //
        // Elevated privileges when container is run - like root.
        Privileged: aws.Bool(false),
        // run commands as this user.
        User: aws.String(""),
        // Labels for SELinux and AppArmour 
        DockerSecurityOptions: []*string{
          aws.String(""),
        },
        // Resource Limits
        //
        // A list of ulimits to set in the container.
        // (eg. CORE, CPU, FSIZE, LOCKS, MLOCK, MSGQUEUE, NICE, NFILE, NPROC, RSS, RTPRIO, RTTIME, SIGPENDING, STACK)
        Ulimits: []*ecs.Ulimit{
          {
            Name: aws.String(""),
            HardLimit: aws.Int64(1),
            SoftLimit: aws.Int64(1),
          },
        },
      },
    },
    Volumes: []*ecs.Volume{
      {
        Host: &ecs.HostVolumeProperties{
          SourcePath: aws.String(""),
        },
        Name: aws.String(""),
      },
    },
  }
  return tdi
}
// func WaitForContainerInstanceStateChange(delaySeconds, periodSeconds int, currentState string, 
//   clusterName string, conatinerIntstanceArn string, ecs_svc *ecs.ECS, cb func(string, error)) {
//   go func() {
//     time.Sleep(time.Second * time.Duration(delaySeconds))
//     var e error
//     var status string
//     for ci, e := s.GetContainerInstanceDescription(); e == nil;  {
//       if *sd.StreamStatus != currentState {
//         status = *sd.StreamStatus
//         break;
//       }
//       time.Sleep(time.Second * time.Duration(periodSeconds))
//       sd, e = s.GetAWSDescription()
//     }
//     cb(status, e)
//   }()
// }

