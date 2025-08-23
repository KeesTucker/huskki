package drivers

const (
	LOG_DIR              = "logs"
	LOG_NAME             = "RAWLOG"
	LOG_EXT              = ".bin"
	WRITE_EVERY_N_FRAMES = 100
)

type Driver interface {
	Init() error
	Run() error
}
