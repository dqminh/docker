package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Regression test for #9414
func TestExecApiCreateNoCmd(t *testing.T) {
	defer deleteAllContainers()
	name := "exec_test"
	runCmd := exec.Command(dockerBinary, "run", "-d", "-t", "--name", name, "busybox", "/bin/sh")
	if out, _, err := runCommandWithOutput(runCmd); err != nil {
		t.Fatal(out, err)
	}

	body, err := sockRequest("POST", fmt.Sprintf("/containers/%s/exec", name), map[string]interface{}{"Cmd": nil})
	if err == nil || !bytes.Contains(body, []byte("No exec command specified")) {
		t.Fatalf("Expected error when creating exec command with no Cmd specified: %q", err)
	}

	logDone("exec create API - returns error when missing Cmd")
}

func TestExecApiStop(t *testing.T) {
	defer deleteAllContainers()
	var (
		cmd  *exec.Cmd
		out  string
		err  error
		name = "exec_api_stop"
	)

	cmd = exec.Command(dockerBinary, "run", "-d", "--name", name, "busybox", "top")
	out, _, err = runCommandWithOutput(cmd)
	if err != nil {
		t.Fatal(out, err)
	}

	execID, err := startExec(name, map[string]interface{}{
		"Cmd": []string{"top", "-b"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// assert that the task is spawned inside the container
	cmd = exec.Command(dockerBinary, "exec", name, "ps")
	out, _, err = runCommandWithOutput(cmd)
	if err != nil {
		t.Fatalf(out, err)
	}
	if !strings.Contains(out, "top -b") {
		t.Fatalf("failed to start exec task. Current processes: %s", out)
	}

	// stop the exec task
	_, err = sockRequest("POST", "/exec/"+execID+"/stop", nil)
	if err != nil {
		fmt.Println("failure, sleep to debug", err)
		time.Sleep(10 * time.Minute)
		t.Fatalf("execStop failed %v", err)
	}

	// assert that we do not have nsenter-exec tasks
	cmd = exec.Command(dockerBinary, "exec", name, "ps")
	out, _, err = runCommandWithOutput(cmd)
	if err != nil {
		t.Fatalf(out, err)
	}
	if strings.Contains(out, "top -b") {
		t.Fatalf("failed to stop exec task. Current processes: %s", out)
	}

	logDone("exec stop API - stop an exec process")
}

// startExec starts an exec session with the provided config and return its ID
func startExec(containerID string, config interface{}) (execID string, err error) {
	endpoint := "/containers/" + containerID + "/exec"
	response, err := sockRequest("POST", endpoint, config)
	if err != nil {
		return "", fmt.Errorf("execCreate failed %v", err)
	}

	// get the exec ID from create response
	var createResp map[string]interface{}
	if err := json.Unmarshal(response, &createResp); err != nil {
		return "", fmt.Errorf("execCreate invalid response %q, %v", string(response), err)
	}
	execID, ok := createResp["Id"].(string)
	if !ok {
		return "", fmt.Errorf("missing Id in response %q", string(response))
	}

	// start the exec task
	endpoint = "/exec/" + execID + "/start"
	_, err = sockRequest("POST", endpoint, map[string]interface{}{
		"Detach": true,
	})
	if err != nil {
		return "", fmt.Errorf("execStart failed %v", err)
	}
	return execID, nil
}
