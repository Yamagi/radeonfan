package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// --------

// Main loop breaks if true.
var quit bool

// The signal handler. To be run as go function.
func sighandler() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
	quit = true
}

// Wrapper function that allows to panic() with a formatted string.
func varpanic(format string, args ...interface{}) {
	msg := fmt.Sprintf("ERROR: "+format+"\n", args...)
	panic(msg)
}

// Returns GPU temperature in degree Celsius.
func gettemp(tempctl *string) int {
	file, err := os.Open(*tempctl)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		varpanic("gettemp %v: couldn't read data", *tempctl)
	}
	rawdegree, err := strconv.Atoi(scanner.Text())
	if err != nil {
		varpanic("gettemp %v: couldn't read data", *tempctl)
	}

	degree := rawdegree / 1000
	if degree < 0 || degree > 119 {
		varpanic("gettemp %v: got %v°C\n", *tempctl, degree)
	}
	return degree
}

// Returns PWM level of fan.
func getpwmspeed(pwmspeedctrl *string) int {
	file, err := os.Open(*pwmspeedctrl)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		varpanic("getpwmspeed %v: couldn't read data", *pwmspeedctrl)
	}
	speed , err := strconv.Atoi(scanner.Text())
	if err != nil {
		varpanic("getpwmspeed %v: couldn't read data", *pwmspeedctrl)
	}
	return speed
}

// Fan modes.
type FanMode int32

const (
	Auto FanMode = iota
	Manual
)

// Sets fan control to auto or manual.
func setpwmmode(mode FanMode, pwmmodectrl *string) {
	file, err := os.OpenFile(*pwmmodectrl, os.O_WRONLY, 0)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	var data string
	if mode == Manual {
		data = "1"
	} else {
		data = "2"
	}
	_, err = file.WriteString(data)
	if err != nil {
		panic(err)
	}
}

// Sets the fan speed to the given PWM level.
func setpwmspeed(pwm int, pwmspeedctrl *string) {
	file, err := os.OpenFile(*pwmspeedctrl, os.O_WRONLY, 0)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Write data,
	data := strconv.Itoa(pwm)
	_, err = file.WriteString(data)
	if err != nil {
		panic(err)
	}
}

// --------

func main() {
	// Register signal handler.
	go sighandler()

	// Die with nicer error messages.
	defer func() {
		if msg := recover(); msg != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", msg)
		}
	}()

	// Must be run as root.
	if user, err := user.Current(); err != nil {
		panic(err)
	} else {
		if strings.Compare(user.Uid, "0") != 0 {
			panic("main: must be run as root")
		}
	}

	// Parse flags.
	var debug = flag.Bool("debug", false, "Debug output")
	var tempctl = flag.String("tempctl", "/sys/class/drm/card0/device/hwmon/hwmon3/temp1_input", "GPU temperature file")
	var pwmmodectrl = flag.String("pwmmodectrl", "/sys/class/drm/card0/device/hwmon/hwmon3/pwm1_enable", "PWM mode control file")
	var pwmspeedctrl = flag.String("pwmspeedctrl", "/sys/class/drm/card0/device/hwmon/hwmon3/pwm1", "PWM speed control file")
	var pwm0 = flag.Int("pwm0", 0, "First PWM point")
	var tmp0 = flag.Int("tmp0", 65, "First temperature point")
	var pwm1 = flag.Int("pwm1", 153, "Second PWM point")
	var tmp1 = flag.Int("tmp1", 80, "Second temperature point")
	var pwm2 = flag.Int("pwm2", 255, "Third PWM point")
	var tmp2 = flag.Int("tmp2", 90, "Third temperature point")
	flag.Parse()

	if *pwm1 < *pwm0 || *pwm2 < *pwm1 {
		panic("main: PWM points must be monotonic, e.g. pwm0 < pwm1 < pwm2")
		if *pwm0 < 0 || *pwm2 > 255 {
			panic("main: PWM points must be between 0 and 225")
		}
	} else if *tmp1 < *tmp0 || *tmp2 < *tmp1 {
		panic("main: temperature points must be monotonic, e.g. tmp0 < tmp1 < tmp2")
		if *tmp0 < 0 || *tmp2 > 119 {
			panic("main: temperature points must be between 0 and 119")
		}
	}

	// Let's precalculate all PWM values for all temperatures
	// between 0 and 119 degrees celcius. Lowest fan speed is
	// 0, highest speed is 255.
	pwmvalues := make([]int, 120)
	for i := 0; i < 120; i++ {
		if i <= *tmp0 {
			pwmvalues[i] = *pwm0
		} else if i >= *tmp2 {
			pwmvalues[i] = *pwm2
		} else {
			if i <= *tmp1 {
				pwmvalues[i] = (i - *tmp0) * (*pwm1 - *pwm0) / (*tmp1 - *tmp0)
			} else {
				pwmvalues[i] = ((i - *tmp1) * (*pwm2 - *pwm1) / (*tmp2 - *tmp1)) + *pwm1
			}
		}
	}

	// Switch fan back to auto mode at exit.
	defer setpwmmode(Auto, pwmmodectrl)

	// Setup state...
	setpwmmode(Manual, pwmmodectrl)
	lasttemp := gettemp(tempctl)
	lastpwm := pwmvalues[lasttemp]
	setpwmspeed(pwmvalues[lasttemp], pwmspeedctrl)

	// ...and get to work.
	for {
		thistemp := gettemp(tempctl)

		// The kernel may have switched us back to automatic mode.
		// (This seems to happen at system suspend / resume)
		setpwmmode(Manual, pwmmodectrl)
		if getpwmspeed(pwmspeedctrl) != lastpwm {
				setpwmspeed(pwmvalues[*tmp0], pwmspeedctrl)
		}

		// We're always increasing if necessary.
		if thistemp > lasttemp {
			if pwmvalues[thistemp] != lastpwm {
				if *debug {
					fmt.Printf("Increasing: %v°C -> %v PWM\n", thistemp, pwmvalues[thistemp])
				}
				setpwmspeed(pwmvalues[thistemp], pwmspeedctrl)
				lasttemp = thistemp
				lastpwm = pwmvalues[thistemp]
			}
		}

		// We're only decreasing if we're 5°C colder.
		if thistemp < (lasttemp - 5) {
			if pwmvalues[thistemp] != lastpwm {
				if *debug {
					fmt.Printf("Decreasing: %v°C -> %v PWM\n", thistemp, pwmvalues[thistemp])
				}
				setpwmspeed(pwmvalues[thistemp], pwmspeedctrl)
				lasttemp = thistemp
				lastpwm = pwmvalues[thistemp]
			}
		}

		if quit {
			break
		}
		time.Sleep(500000000)
	}
}