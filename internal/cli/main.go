package cli

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"

	"github.com/rarimo/evm-identity-saver-svc/internal/services/grpc"
	"github.com/rarimo/evm-identity-saver-svc/internal/services/voting"

	"github.com/rarimo/evm-identity-saver-svc/internal/services/evm"

	"github.com/alecthomas/kingpin"
	"github.com/rarimo/evm-identity-saver-svc/internal/config"
	"gitlab.com/distributed_lab/kit/kv"
)

func Run(args []string) bool {
	log := logan.New()

	defer func() {
		if rvr := recover(); rvr != nil {
			log.WithRecover(rvr).Error("app panicked")
		}
	}()

	cfg := config.New(kv.MustFromEnv())
	log = cfg.Log()

	app := kingpin.New("evm-identity-saver-svc", "")

	runCmd := app.Command("run", "run command")

	apiCmd := runCmd.Command("api", "run grpc api")

	stateUpdateAll := runCmd.Command("state-update-all", "run all state update related services")
	stateUpdateVoter := runCmd.Command("state-update-voter", "run state update voter")
	stateUpdateSaver := runCmd.Command("state-update-saver", "run state update saver")

	cmd, err := app.Parse(args[1:])
	if err != nil {
		log.WithError(err).Error("failed to parse arguments")
		return false
	}

	var wg sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	run := func(f func(ctx context.Context, cfg config.Config), name string) {
		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()
				cfg.Log().WithField("who", name).Info("finished routine")
			}()

			cfg.Log().WithField("who", name).Info("starting routine")
			f(ctx, cfg)
		}()
	}

	runStateUpdatesAll := func() {
		cfg.Log().Info("starting all state updates related services")

		run(voting.RunStateUpdateVoter, "voter")
		run(grpc.RunAPI, "grpc-api")
		run(evm.RunStateChangeListener, "state-change-listener")
	}

	if profiler := cfg.Profiler(); profiler.Enabled {
		profiler.RunProfiling()
	}

	switch cmd {
	case apiCmd.FullCommand():
		run(grpc.RunAPI, "grpc-api")
	case stateUpdateAll.FullCommand():
		runStateUpdatesAll()
	case stateUpdateVoter.FullCommand():
		run(voting.RunStateUpdateVoter, "voter")
	case stateUpdateSaver.FullCommand():
		run(evm.RunStateChangeListener, "state-change-listener")
	default:
		panic(errors.From(errors.New("unknown command"), logan.F{
			"raw_command": cmd,
		}))
	}

	gracefulStop := make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGTERM, syscall.SIGINT)

	wgch := make(chan struct{})
	go func() {
		wg.Wait()
		close(wgch)
	}()

	select {
	case <-wgch:
		cfg.Log().Warn("all services stopped")
	case <-gracefulStop:
		cfg.Log().Info("received signal to stop")
		cancel()
		<-wgch
	}

	return true
}
