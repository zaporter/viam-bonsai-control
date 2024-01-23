# viam-bonsai-control
Specialized module to control my bonsai tree. Not useful for other people



Config:
```golang
type Config struct {
	PumpPin              int    `json:"pump_pin"`
	SensePin             int    `json:"sense_pin"`
	WaterIntervalSeconds int    `json:"water_interval_seconds"`
	WaterDurationSeconds int    `json:"water_duration_seconds"`
	BoardComponent       string `json:"board"`
}
```
