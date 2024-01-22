// Package sds011 is the package for sds011
package sds011

import (
	"context"

	"github.com/pkg/errors"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

var (
	Model = resource.NewModel("zaporter", "bonsai", "v1")
)

func init() {
	registration := resource.Registration[resource.Resource, *Config]{
		Constructor: func(ctx context.Context,
			deps resource.Dependencies,
			conf resource.Config,
			logger logging.Logger,
		) (resource.Resource, error) {
			return createComponent(ctx, deps, conf, logger, false)
		},
	}
	resource.RegisterComponent(sensor.API, Model, registration)
}

type component struct {
	resource.Named
	resource.AlwaysRebuild
	cfg *Config

	cancelCtx  context.Context
	cancelFunc func()

	logger logging.Logger
}

func createComponent(_ context.Context,
	_ resource.Dependencies,
	conf resource.Config,
	logger logging.Logger,
	isFake bool,
) (sensor.Sensor, error) {
	newConf, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return nil, errors.Wrap(err, "create component failed due to config parsing")
	}

	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	instance := &component{
		Named:      conf.ResourceName().AsNamed(),
		cfg:        newConf,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
		logger:     logger,
	}
	return instance, nil
}

func (c *component) Readings(ctx context.Context, extra map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"pm_10":  10.0,
		"pm_2.5": 15.0,
		"units":  "μg/m³",
	}, nil
}

// DoCommand sends/receives arbitrary data.
func (c *component) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return make(map[string]interface{}), nil
}

// Close must safely shut down the resource and prevent further use.
// Close must be idempotent.
// Later reconfiguration may allow a resource to be "open" again.
func (c *component) Close(ctx context.Context) error {
	c.cancelFunc()
	c.logger.Info("closing\n")
	return nil
}
