// Package sds011 is the package for sds011
package sds011

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/utils"
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

	boardComponent board.Board
	cancelCtx      context.Context
	cancelFunc     func()

	logger logging.Logger
}

func createComponent(_ context.Context,
	deps resource.Dependencies,
	conf resource.Config,
	logger logging.Logger,
	isFake bool,
) (sensor.Sensor, error) {
	newConf, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return nil, errors.Wrap(err, "create component failed due to config parsing")
	}
	board, err := board.FromDependencies(deps, newConf.BoardComponent)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get board component")
	}

	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	instance := &component{
		Named:          conf.ResourceName().AsNamed(),
		cfg:            newConf,
		boardComponent: board,
		cancelCtx:      cancelCtx,
		cancelFunc:     cancelFunc,
		logger:         logger,
	}
	instance.startBgProcess()
	return instance, nil
}

func (c *component) startBgProcess() {
	c.logger.Info("starting a watering\n")
	err := c.water()
	if err != nil {
		c.logger.Errorw("error watering", "err", err)
	}
	utils.PanicCapturingGo(func() {
		ticker := time.NewTicker(time.Second * time.Duration(c.cfg.WaterIntervalSeconds))
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.logger.Info("starting a watering\n")
				err := c.water()
				if err != nil {
					c.logger.Errorw("error watering", "err", err)
				}
			case <-c.cancelCtx.Done():
				c.logger.Info("shutdown")
				return
			}
		}
	})
}

func (c *component) water() error {
	sensePin, err := c.boardComponent.GPIOPinByName(string(c.cfg.SensePin))
	if err != nil {
		return errors.Wrap(err, "sensepin")
	}
	pumpPin, err := c.boardComponent.GPIOPinByName(string(c.cfg.PumpPin))
	if err != nil {
		return errors.Wrap(err, "pumppin")
	}

	// make sure we set low at the end even in an error
	defer pumpPin.Set(context.Background(), false, nil)

	startTime := time.Now()
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	for time.Since(startTime) < time.Second*time.Duration(c.cfg.WaterDurationSeconds) {
		select {
		case <-ticker.C:
			c.logger.Info("watering")
			senseVal, err := sensePin.Get(c.cancelCtx, nil)
			if err != nil {
				c.logger.Errorw("error reading sense", "err", err)
			}
			// if low, make sure the pump pin is high, else set low
			if senseVal {
				err = pumpPin.Set(context.Background(), false, nil)
				if err != nil {
					// TODO: power off the device.
					// if we don't have control of the pump, the pi could be damaged
					return errors.Wrap(err, "failed to set pump pin to low")
				}
			} else {
				err = pumpPin.Set(context.Background(), true, nil)
				if err != nil {
					// TODO: power off the device.
					// if we don't have control of the pump, the pi could be damaged
					return errors.Wrap(err, "failed to set pump pin to low")
				}
			}
		case <-c.cancelCtx.Done():
			c.logger.Info("shutdown")
			return c.cancelCtx.Err()
		}
	}

	err = pumpPin.Set(context.Background(), false, nil)
	if err != nil {
		// TODO: power off the device.
		// if we don't have control of the pump, the pi could be damaged
		return errors.Wrap(err, "failed to set pump pin to low")
	}
	return nil
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
