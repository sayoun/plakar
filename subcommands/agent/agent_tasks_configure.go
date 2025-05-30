package agent

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/PlakarKorp/plakar/appcontext"
	"github.com/PlakarKorp/kloset/repository"
	"github.com/PlakarKorp/plakar/scheduler"
	"github.com/PlakarKorp/plakar/subcommands"
)

type AgentTasksConfigure struct {
	subcommands.SubcommandBase

	ConfigurationFile string
}

func (cmd *AgentTasksConfigure) Parse(ctx *appcontext.AppContext, args []string) error {
	flags := flag.NewFlagSet("agent tasks configure", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [OPTIONS]\n", flags.Name())
		fmt.Fprintf(flags.Output(), "\nOPTIONS:\n")
		flags.PrintDefaults()
	}
	flags.Parse(args)

	if flags.NArg() == 0 {
		flags.Usage()
		return fmt.Errorf("no configuration file provided")
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("too many arguments")
	}

	if configurationFile, err := filepath.Abs(flags.Arg(0)); err != nil {
		return fmt.Errorf("failed to get absolute path for configuration file: %w", err)
	} else {
		cmd.ConfigurationFile = configurationFile
	}

	return nil
}

func (cmd *AgentTasksConfigure) Execute(ctx *appcontext.AppContext, repo *repository.Repository) (int, error) {
	if agentContextSingleton == nil {
		return 1, fmt.Errorf("agent not started")
	}

	schedConfig, err := scheduler.ParseConfigFile(cmd.ConfigurationFile)
	if err != nil {
		return 1, err
	}

	agentContextSingleton.mtx.Lock()
	defer agentContextSingleton.mtx.Unlock()

	if agentContextSingleton.schedulerCtx != nil {
		fmt.Fprintf(ctx.Stderr, "Reloading agent scheduler... (may take some time)\n")
		agentContextSingleton.schedulerCtx.Cancel()
		agentContextSingleton.schedulerCtx = appcontext.NewAppContextFrom(agentContextSingleton.agentCtx)

		go scheduler.NewScheduler(agentContextSingleton.schedulerCtx, schedConfig).Run()

		fmt.Fprintf(ctx.Stderr, "done !\n")
	}

	agentContextSingleton.schedulerConfig = schedConfig
	return 0, nil
}
