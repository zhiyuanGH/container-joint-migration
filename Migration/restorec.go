package Migration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
	"github.com/docker/go-connections/nat"


	"github.com/docker/docker/api/types/container"

	"github.com/docker/docker/client"
)

// PullImageIfNotExists pulls the specified image if it does not exist locally

func RestoreContainer(checkpointData []byte, image string, name string, binds string) (newContainerID string, DurationCreateFS time.Duration, DurationExtractCheckpoint time.Duration, err error) {
	fmt.Printf("Starting restore container %s with image %s\n", name, image)
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", time.Duration(0), time.Duration(0), fmt.Errorf("error creating Docker client: %v", err)
	}

	// Handle binds: If binds is empty, pass nil to HostConfig.Binds
	var bindList []string
	if binds != "" {
		bindList = append(bindList, binds)
	}

	// Create the container
	startTime := time.Now()
    exposedPorts := nat.PortSet{
        "6379/tcp": struct{}{}, // Expose port 6379 for TCP
    }

    // // Define the port bindings (hostPort:containerPort)
    // portBindings := nat.PortMap{
    //     "6379/tcp": []nat.PortBinding{
    //         {
    //             HostIP:   "0.0.0.0", // Bind to all interfaces
    //             HostPort: "6379",    // Host port
    //         },
    //     },
	// }


    // // Define resource constraints
    // memoryLimit := int64(2 * 1024 * 1024 * 1024) // 2GB in bytes
    // cpuLimit := int64(2 * 1e9)                    // 2 CPUs in nanoCPUs



	newResp, err := cli.ContainerCreate(context.Background(), &container.Config{
		Image: image,
		Tty:   false,
		ExposedPorts: exposedPorts,
		

	}, &container.HostConfig{
		Binds: bindList, // Use bindList which is nil if binds was empty
		// PortBindings: portBindings,
		// Resources: container.Resources{
		// 	Memory: memoryLimit,
		// 	NanoCPUs: cpuLimit,
		// },

	}, nil, nil, name)
	if err != nil {
		return "", time.Duration(0), time.Duration(0), fmt.Errorf("error creating container: %v", err)
	}
	fmt.Printf("Created container with ID: %s and Name: %s\n", newResp.ID, name)
	DurationCreateFS = time.Since(startTime)
	fmt.Printf("Create container snapshot duration: %v\n", DurationCreateFS)


	startTimeCheckpoint := time.Now()
	// Create checkpoint directory
	checkpointDir := fmt.Sprintf("/var/lib/docker/containers/%s/checkpoints/checkpoint1", newResp.ID)
	err = os.MkdirAll(checkpointDir, os.ModePerm)
	if err != nil {
		return "", time.Duration(0), time.Duration(0), fmt.Errorf("error creating checkpoint directory: %v", err)
	}
	fmt.Print("Created checkpoint directory successfully\n")

	// Unzip the checkpoint data
	buf := bytes.NewBuffer(checkpointData)
	gz, err := gzip.NewReader(buf)
	if err != nil {
		return "", time.Duration(0), time.Duration(0), fmt.Errorf("error creating gzip reader for checkpoint data: %v", err)
	}
	tarReader := tar.NewReader(gz)

	// Extract the checkpoint data
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", time.Duration(0), time.Duration(0), fmt.Errorf("error reading tar header: %v", err)
		}

		target := filepath.Join(checkpointDir, hdr.Name)
		if hdr.Typeflag == tar.TypeDir {
			err = os.MkdirAll(target, os.ModePerm)
			if err != nil {
				return "", time.Duration(0), time.Duration(0), fmt.Errorf("error creating directory in checkpoint: %v", err)
			}
		} else {
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return "", time.Duration(0), time.Duration(0), fmt.Errorf("error opening file in checkpoint: %v", err)
			}
			_, err = io.Copy(f, tarReader)
			if err != nil {
				return "", time.Duration(0), time.Duration(0), fmt.Errorf("error copying data to file in checkpoint: %v", err)
			}
			f.Close()
		}
	}
	fmt.Println("Extracted checkpoint data successfully")

	// Start the container with the checkpoint
	err = cli.ContainerStart(context.Background(), newResp.ID, container.StartOptions{CheckpointID: "checkpoint1"})
	if err != nil {
		return "", time.Duration(0), time.Duration(0), fmt.Errorf("error starting container: %v", err)
	}
	DurationExtractCheckpoint = time.Since(startTimeCheckpoint)
	fmt.Printf("Extract checkpoint duration: %v\n", DurationExtractCheckpoint)
	fmt.Printf("Container started successfully with ID: %s\n", newResp.ID)

	return newResp.ID, DurationCreateFS, DurationExtractCheckpoint, nil
}

// currently PullContainerToLocalhost is more like to fetch a container from given address to local host
