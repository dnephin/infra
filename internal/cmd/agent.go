package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/spf13/cobra"

	"github.com/infrahq/infra/internal"
	"github.com/infrahq/infra/internal/logging"
	"github.com/infrahq/infra/internal/repeat"
)

var countCtxKey = key{}

type counter struct {
	count     int
	threshold int
}

func newAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "agent",
		Short:  "Start the Infra agent",
		Long:   "Start the Infra agent that runs to sync Infra state",
		Args:   NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			infraDir, err := initInfraHomeDir()
			if err != nil {
				panic(err)
			}

			logging.UseFileLogger(filepath.Join(infraDir, "agent.log"))

			var wg sync.WaitGroup

			// start background tasks
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ctx = context.WithValue(ctx, countCtxKey, &counter{count: 0, threshold: 3})

			repeat.InGroup(&wg, ctx, cancel, 1*time.Minute, syncKubeConfig)
			// add the next agent task here

			logging.Infof("starting infra agent (%s)", internal.FullVersion())

			wg.Wait()

			return ctx.Err()
		},
	}
}

// configAgentRunning checks if the agent process stored in config is still running
func configAgentRunning() (bool, error) {
	pid, err := readStoredAgentProcessID()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// this is the first time the agent is running, suppress the error and continue
			logging.Debugf("%s", err.Error())
			return false, nil
		}
		return false, err
	}

	return processRunning(int32(pid))
}

func processRunning(pid int32) (bool, error) {
	if pid == 0 { // on windows pid 0 is the system idle process, it will be running but its not the agent
		return false, nil
	}

	running, err := process.PidExists(pid)
	if err != nil {
		return false, err
	}

	return running, nil
}

// execAgent executes the agent in the background and stores its process ID in configuration
func execAgent() error {
	// find the location of the infra executable running
	// this ensures we run the agent for the correct version
	infraExe, err := os.Executable()
	if err != nil {
		return err
	}

	// use the current infra executable to start the agent
	cmd := exec.Command(infraExe, "agent")
	if err := cmd.Start(); err != nil {
		return err
	}

	logging.Debugf("agent started, pid: %d", cmd.Process.Pid)

	return writeAgentConfig(cmd.Process.Pid)
}

func readStoredAgentProcessID() (int, error) {
	infraDir, err := initInfraHomeDir()
	if err != nil {
		return 0, err
	}

	agentConfig, err := os.Open(filepath.Join(infraDir, "agent.pid"))
	if err != nil {
		return 0, err
	}
	defer agentConfig.Close()

	var pid int

	_, err = fmt.Fscanf(agentConfig, "%d\n", &pid)
	if err != nil {
		return 0, err
	}

	return pid, nil
}

// writeAgentProcessConfig saves details about the agent to config
func writeAgentConfig(pid int) error {
	infraDir, err := initInfraHomeDir()
	if err != nil {
		return err
	}

	agentConfig, err := os.Create(filepath.Join(infraDir, "agent.pid"))
	if err != nil {
		return err
	}
	defer agentConfig.Close()

	_, err = agentConfig.WriteString(fmt.Sprintf("%d\n", pid))
	if err != nil {
		return err
	}

	return nil
}

func maybeCancel(msg string, err error, count *counter, cancel context.CancelFunc) {
	count.count++
	logging.L.Error().Err(err).Int("count", count.count).Int("threshold", count.threshold).Msg(msg)
	if count.count > count.threshold {
		cancel()
	}
}

// syncKubeConfig updates the local kubernetes configuration from Infra grants
func syncKubeConfig(ctx context.Context, cancel context.CancelFunc) {
	start := time.Now()
	count, ok := ctx.Value(countCtxKey).(*counter)
	if !ok {
		logging.L.Error().Msg("failed to get counter")
		cancel()
	}

	client, err := defaultAPIClient()
	if err != nil {
		maybeCancel("failed to get client", err, count, cancel)
		return
	}
	client.Name = "agent"

	user, destinations, grants, err := getUserDestinationGrants(client)
	if err != nil {
		maybeCancel("failed to get grants", err, count, cancel)
		return
	}

	if err := writeKubeconfig(user, destinations, grants); err != nil {
		maybeCancel("failed to update kubeconfig", err, count, cancel)
		return
	}

	logging.L.Info().Dur("elapsed", time.Since(start)).Msg("finished kubeconfig sync")
	count.count = 0
}
