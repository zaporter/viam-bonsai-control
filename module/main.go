package main

import (
	"context"

	mymodule "github.com/zaporter/viam-bonsai-control"
	"go.viam.com/utils"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/module"
)

var (
	Version     = "development"
	GitRevision = ""
)

func main() {
	utils.ContextualMain(mainWithArgs, module.NewLoggerFromArgs("bonsai-control"))
}

func mainWithArgs(ctx context.Context, args []string, logger logging.Logger) error {
	var versionFields []interface{}
	if Version != "" {
		versionFields = append(versionFields, "version", Version)
	}
	if GitRevision != "" {
		versionFields = append(versionFields, "git_rev", GitRevision)
	}
	if len(versionFields) != 0 {
		logger.Infow("bonsai-contol", versionFields...)
	} else {
		logger.Info("bonsai-control" + " built from source; version unknown")
	}
	mod, err := module.NewModuleFromArgs(ctx, logger)
	if err != nil {
		return err
	}
	if err := mod.AddModelFromRegistry(ctx, sensor.API, mymodule.Model); err != nil {
		return err
	}
	if err := mod.Start(ctx); err != nil {
		return err
	}
	defer mod.Close(ctx)
	<-ctx.Done()
	return nil
}
