package sds011

import (
	"errors"
)

type Config struct {
	PumpPin              int    `json:"pump_pin"`
	SensePin             int    `json:"sense_pin"`
	WaterIntervalSeconds int    `json:"water_interval_seconds"`
	WaterDurationSeconds int    `json:"water_duration_seconds"`
	BoardComponent       string `json:"board"`
}

// Validate takes the current location in the config (useful for good error messages).
// It should return a []string which contains all of the implicit
// dependencies of a module. (or nil,err if the config does not pass validation).
func (cfg *Config) Validate(path string) ([]string, error) {
	if cfg.PumpPin == 0 || cfg.SensePin == 0 {
		return nil, errors.New(path + " pump_pin and sense_pin must be non-empty")
	}
	if cfg.WaterIntervalSeconds == 0 || cfg.WaterDurationSeconds == 0 {
		return nil, errors.New(path + " water_interval_seconds and water_duration_seconds must be non-empty")
	}
	if cfg.BoardComponent == "" {
		return nil, errors.New(path + " board must be non-empty")
	}
	return []string{cfg.BoardComponent}, nil
}
