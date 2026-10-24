package utils

var (
	initialized bool
)

func Init() error {
	if initialized {
		return nil
	}

	err := InitStorage()
	if err != nil {
		return err
	}

	config := GetConfig()
	err = config.Load()
	if err != nil {
		return err
	}
	err = config.Save()
	if err != nil {
		return err
	}

	initialized = true
	return nil
}

func init() {
	if err := Init(); err != nil {
		panic(err)
	}
}
