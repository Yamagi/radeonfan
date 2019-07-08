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

// Returns PWM speed.
func getpwmspeed(pwmctl string) int {
	file, err := os.Open(pwmctl)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		varpanic("getpwmspeed %v: couldn't read data", pwmctl)
	}
	pwmspeed, err := strconv.Atoi(scanner.Text())
	if err != nil {
		varpanic("getpwmspeed %v: couldn't read data", pwmctl)
	}

	if pwmspeed < 0 || pwmspeed > 255 {
		varpanic("gettemp %v: got %v\n", pwmctl, pwmspeed)
	}
	return pwmspeed
}

// Returns GPU temperature in degree Celsius.
func gettemp(tempctl string) int {
	file, err := os.Open(tempctl)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		varpanic("gettemp %v: couldn't read data", tempctl)
	}
	rawdegree, err := strconv.Atoi(scanner.Text())
	if err != nil {
		varpanic("gettemp %v: couldn't read data", tempctl)
	}

	degree := rawdegree / 1000
	if degree < 0 || degree > 119 {
		varpanic("gettemp %v: got %v°C\n", tempctl, degree)
	}
	return degree
}

// Returns PWM control mode.
func getpwmmode(pwmmodectrl string) FanMode {
	file, err := os.Open(pwmmodectrl)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		varpanic("getpwmmode %v: couldn't read data", pwmmodectrl)
	}
	mode , err := strconv.Atoi(scanner.Text())
	if err != nil {
		varpanic("getpwmmode %v: couldn't read data", pwmmodectrl)
	}
	return FanMode(mode)
}

// Fan modes.
type FanMode int32

const (
	Auto FanMode = iota
	Manual
)

// Sets fan control to auto or manual.
func setpwmmode(mode FanMode, pwmmodectrl string) {
	file, err := os.OpenFile(pwmmodectrl, os.O_WRONLY, 0)
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
func setpwmspeed(pwm int, pwmspeedctrl string) {
	file, err := os.OpenFile(pwmspeedctrl, os.O_WRONLY, 0)
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
	var card = flag.String("card", "card0", "Card to control")
	var pwm0 = flag.Int("pwm0", 0, "First PWM point")
	var tmp0 = flag.Int("tmp0", 50, "First temperature point")
	var pwm1 = flag.Int("pwm1", 153, "Second PWM point")
	var tmp1 = flag.Int("tmp1", 75, "Second temperature point")
	var pwm2 = flag.Int("pwm2", 255, "Third PWM point")
	var tmp2 = flag.Int("tmp2", 85, "Third temperature point")
	flag.Parse()

	var ctrldir string
	for i := 0; i <=100 ; i++ {
		testdir := fmt.Sprintf("/sys/class/drm/%v/device/hwmon/hwmon%v/", *card, i)
		if fi, err := os.Stat(testdir); err == nil {
			if fi.IsDir() {
				ctrldir = testdir
				break
			}
		}
	}
	if len(ctrldir) == 0 {
		varpanic("main: no hwmon for %v found", *card)
	}

	tempctl := fmt.Sprintf("%s/temp1_input", ctrldir)
	pwmmodectrl := fmt.Sprintf("%s/pwm1_enable", ctrldir)
	pwmspeedctrl := fmt.Sprintf("%s/pwm1", ctrldir)

	pwmmin := getpwmspeed(fmt.Sprintf("%s/pwm1_min", ctrldir))
	pwmmax := getpwmspeed(fmt.Sprintf("%s/pwm1_max", ctrldir))

	if *pwm1 < pwmmin || *pwm2 > pwmmax {
		varpanic("main: PWM points must be within the PWM range (%v to %v) of your card", pwmmin, pwmmax)
	}

	tempcrit := gettemp(fmt.Sprintf("%s/temp1_crit", ctrldir))

	if *tmp2 > (tempcrit - 5) {
		varpanic("main: -tmp2 must be 5°C less than the critical temperature (%v°C) of your card", tempcrit)
	}

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
	lasttemp := gettemp(tempctl)
	if *debug {
		fmt.Printf("Initializing: %v°C -> %v PWM\n", lasttemp, pwmvalues[lasttemp])
	}
	if getpwmmode(pwmmodectrl) != Manual {
		setpwmmode(Manual, pwmmodectrl)
	}
	setpwmspeed(pwmvalues[lasttemp], pwmspeedctrl)

	// ...and get to work.
	for {
		thistemp := gettemp(tempctl)

		// The kernel may have switched us back to automatic mode.
		// (This seems to happen at system suspend / resume)
		if getpwmmode(pwmmodectrl) != Manual {
			if *debug {
				fmt.Printf("Renitializing: %v°C -> %v PWM\n", thistemp, pwmvalues[thistemp])
			}
			setpwmmode(Manual, pwmmodectrl)
			setpwmspeed(pwmvalues[thistemp], pwmspeedctrl)
			lasttemp = thistemp
		}

		// We're always increasing if necessary.
		// Give full speed if we're less then 5°C
		// under the critical temperature.
		if thistemp > (tempcrit - 5) {
			if *debug {
				fmt.Printf("Overheating: %v°C -> %v PWM\n", thistemp, pwmmax)
			}
			setpwmspeed(pwmmax, pwmspeedctrl)
			lasttemp = thistemp
		} else if thistemp > lasttemp {
			if pwmvalues[thistemp] != pwmvalues[lasttemp] {
				if *debug {
					fmt.Printf("Increasing: %v°C -> %v PWM\n", thistemp, pwmvalues[thistemp])
				}
				setpwmspeed(pwmvalues[thistemp], pwmspeedctrl)
				lasttemp = thistemp
			}
		}

		// We're only decreasing if we're 5°C colder.
		if thistemp < (lasttemp - 5) {
			if pwmvalues[thistemp] != pwmvalues[lasttemp] {
				if *debug {
					fmt.Printf("Decreasing: %v°C -> %v PWM\n", thistemp, pwmvalues[thistemp])
				}
				setpwmspeed(pwmvalues[thistemp], pwmspeedctrl)
				lasttemp = thistemp
			}
		}

		if quit {
			break
		}
		time.Sleep(500000000)
	}
}
