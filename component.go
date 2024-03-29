// Package sds011 is the package for sds011
package sds011

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/utils"
	"image"
	"image/color"
	"image/draw"

	"github.com/waxdred/go-i2c-oled"
	"github.com/waxdred/go-i2c-oled/ssd1306"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var (
	Model   = resource.NewModel("zaporter", "bonsai", "v1")
	DataDir = os.Getenv("VIAM_MODULE_DATA")
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

	logger        logging.Logger
	isWatering    bool
	wateringStart time.Time
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
		isWatering:     false,
	}

	if err := ensureNextWaterTime(time.Now().Add(time.Second * time.Duration(instance.cfg.WaterIntervalSeconds))); err != nil {
		// it is dangerous to not set the next water time otherwise we could continuously water
		instance.logger.Fatalw("error setting next water time", "err", err)
	}
	instance.startBgProcess()
	return instance, nil
}

func (c *component) startBgProcess() {
	utils.PanicCapturingGo(func() {
		// check every 5 seconds
		ticker := time.NewTicker(time.Second * 5)

		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-ticker.C:
				i += 1
				if i%2 == 0 {
					if err := c.PushStats(); err != nil {
						c.logger.Errorw("error pushing stats", "err", err)
					}
				}
				nextWaterTime, err := readNextTime()
				if err != nil {
					c.logger.Errorw("error reading next water time", "err", err)
					continue
				}
				if nextWaterTime.After(time.Now()) {
					continue
				}

				c.logger.Info("starting a watering\n")
				c.isWatering = true
				if err := c.water(); err != nil {
					c.logger.Errorw("error watering", "err", err)
				}
				c.isWatering = false
				if err := writeNextTime(nextWaterTime.Add(time.Second * time.Duration(c.cfg.WaterIntervalSeconds))); err != nil {
					// it is dangerous to not set the next water time otherwise we could continuously water
					c.logger.Fatalw("error setting next water time", "err", err)
				}
			case <-c.cancelCtx.Done():
				c.logger.Info("shutdown")
				return
			}
		}
	})
}

func writeNextTime(nextTime time.Time) error {
	return os.WriteFile(filepath.Join(DataDir, "time.txt"), []byte(nextTime.Format(time.RFC3339)), 0o700)
}

func readNextTime() (time.Time, error) {
	contents, err := os.ReadFile(filepath.Join(DataDir, "time.txt"))
	if err != nil {
		return time.Now(), err
	}
	return time.Parse(time.RFC3339, string(contents))
}

// if the file doesn't exist, write it
func ensureNextWaterTime(nextTime time.Time) error {
	if _, err := readNextTime(); err != nil {
		return writeNextTime(nextTime)
	}
	return nil
}

func (c *component) water() error {
	sensePin, err := c.boardComponent.GPIOPinByName(fmt.Sprint(c.cfg.SensePin))
	if err != nil {
		return errors.Wrap(err, "sensepin")
	}
	pumpPin, err := c.boardComponent.GPIOPinByName(fmt.Sprint(c.cfg.PumpPin))
	if err != nil {
		return errors.Wrap(err, "pumppin")
	}

	c.wateringStart = time.Now()
	c.logger.Info("starting to water")
	// make sure we set low at the end even in an error
	defer pumpPin.Set(context.Background(), false, nil)

	startTime := time.Now()
	ticker := time.NewTicker(time.Millisecond * 100)
	i := 0
	defer ticker.Stop()
	for time.Since(startTime) < time.Second*time.Duration(c.cfg.WaterDurationSeconds) {
		i += 1
		if i%20 == 0 {
			if err := c.PushStats(); err != nil {
				c.logger.Errorw("error pushing stats", "err", err)
			}
			c.logger.Infof("Time watered: %v. Time left: %v", time.Since(startTime), time.Second*time.Duration(c.cfg.WaterDurationSeconds)-time.Since(startTime))
		}
		select {
		case <-ticker.C:
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
	if c.isWatering {
		return map[string]interface{}{
			"water time left": ((time.Second * time.Duration(c.cfg.WaterDurationSeconds)) - time.Since(c.wateringStart)).Round(time.Second).String(),
		}, nil

	} else {
		nextWaterTime, err := readNextTime()
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"next water time":      nextWaterTime.Format(time.RFC1123),
			"time till next water": time.Until(nextWaterTime).Round(time.Second).String(),
		}, nil
	}
}

func (c *component) PushStats() error {
	if c.isWatering {
		return drawToDisplay("Time left:", ((time.Second * time.Duration(c.cfg.WaterDurationSeconds)) - time.Since(c.wateringStart)).Round(time.Second).String())

	} else {
		nextWaterTime, err := readNextTime()
		if err != nil {
			return err
		}
		return drawToDisplay("Time till water:", time.Until(nextWaterTime).Round(time.Second).String())
	}
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

	// ensure we turn off the pump in case it is accidentally still on
	pumpPin, err := c.boardComponent.GPIOPinByName(fmt.Sprint(c.cfg.PumpPin))
	if err != nil {
		return errors.Wrap(err, "pump pin")
	}
	err = pumpPin.Set(context.Background(), false, nil)
	if err != nil {
		// TODO: power off the device.
		// if we don't have control of the pump, the pi could be damaged
		return errors.Wrap(err, "failed to set pump pin to low")
	}
	c.logger.Info("closing\n")
	return nil
}

func drawToDisplay(line1 string, line2 string) error {

	// Initialize the OLED display with the provided parameters
	oled, err := goi2coled.NewI2c(ssd1306.SSD1306_SWITCHCAPVCC, 32, 128, 0x3C, 1)
	if err != nil {
		panic(err)
	}

	defer oled.Close()

// 	// Define a black color
// 	black := color.RGBA{0, 0, 0, 255}

// 	// Set the entire OLED image to black
// 	draw.Draw(oled.Img, oled.Img.Bounds(), &image.Uniform{black}, image.Point{}, draw.Src)

	// Define a white color
	colWhite := color.RGBA{255, 255, 255, 255}

	// Set the starting point for drawing text
	point := fixed.Point26_6{fixed.Int26_6(0 * 64), fixed.Int26_6(15 * 64)} // x = 0, y = 15

	// Configure the font drawer with the chosen font and color
	drawer := &font.Drawer{
		Dst:  oled.Img,
		Src:  &image.Uniform{colWhite},
		Face: basicfont.Face7x13,
		Dot:  point,
	}

	// Clear the OLED image (making it all black)
	draw.Draw(oled.Img, oled.Img.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)

	// Draw the text "Hello" on the OLED image
	drawer.DrawString(line1)

	// Move the drawing point down by 10 pixels for the next line of text
	drawer.Dot.Y += fixed.Int26_6(10 * 64)

	// Set the drawing point's x coordinate back to 0 for alignment
	drawer.Dot.X = fixed.Int26_6(0 * 64)

	// Draw the text "From golang!" on the OLED image
	drawer.DrawString(line2)

	// Clear the OLED's buffer (if applicable to your library)
	oled.Clear()

	// Update the OLED's buffer with the current image data
	oled.Draw()

	// Display the buffered content on the OLED screen
	err = oled.Display()
	return err
}
