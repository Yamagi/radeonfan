# Fan control daemon for Radeon GPUs

This a daemon to control the fan speed of Radeon video cards. Currently
it's only tested with an Radeon RX 580, it should work with all cards /
GPUs supported by the `amdgpu` driver.

**ATTENTION**: Changing the fan speeds of your video card may overheat
or even destroy your card! This tool comes without any warranty. If it
destroys your hardware, it's your and only your problem!


## Inner Workings

All GCN and RDNA based Radeon video cards offer two ways of fan control:

* **Firmware**: The fan is controlled by logic implemented in the video
  cards firmware. This mode is usually extremely conservative, most
  cards start at about 30% PWM and max out at 100% PWM somewhere around
  75°C.
* **Software**: The fan is controlled by software. The video cards
  provides a fan curve to the software, but the software may ignore it
  and do whatever it wants.

The card starts in firmware mode. Under Windows the driver switches it
to software mode as soon as the system has booted to the desktop. The
Linux driver has no software fan control, so the card stays in firmware
mode.

The Windows driver is rather conservative. It reads three tuples of GPU
temperature and corresponding PWM speed from the cards ROM. This is the
default fan curve: `(tmp0,pwm0)`, `(tmp1,pwm1)` and `(tmp2,pwm2)` For
temperatures up to `tmp0` the fan stays at `pwm0`, for temperatures
between `tmp0` and `tmp1` at `pwm1`, etc. The fan speeds up as soon as a
temperature point is reached and slows down to the next slower level if
the temperature falls about 15°C to 20°C under the next lower
temperature point.

We're taking a more aggressive approach: Up to `tmp0` the fan stays as
`pwm0`. Between `tmp0` and `tmp1` it's set to linear interpolated values
between `pwm0` and `pwm1`, the same goes to `tmp1` and `tmp2`. Above
`tmp2` it's always set to `pwm2`. The fan speed is increased as the
temperature rises and only decreased if the temperature has fallen by at
least 5°C. With this, the card is less noisy then under Windows, but the
temperatures are comparable.

There's one safety measure: The cards ROM defines a maximum temperature.
If the GPU reaches this temperature, it will throttle to protect itself
from damage. We're driving the fan at full speed if the GPU temperature
is less than 5°C under the maximum temperature, regardless of the fan
curve.


## Installation

If your distro has a package you likely want to use that. A precompiled
binary of the latest release can be found under the *release* tab at
Github.  To compile the program by yourself:

* You'll need `go` 1.12 or higher.
* Run `go get github.com/yamagi/radeonfan/cmd/radeonfan`.
* The binary ca be found at `$HOME/go/bin/radeonfan`.

Copy the binary were you want it and alter `init/radeonfan.service` to
suit your needs. Copy it to `/etc/systemd/system`, enable and start it
as usual.


## Command Line Arguments

* **-card**: Card to control, defaults to `card0`.
* **-debug**: Print fan speed changes.
* **-pwm0** and **-tmp0**: First temperature / PWM tuple.
* **-pwm1** and **-tmp1**: Second temperature / PWM tuple.
* **-pwm2** and **-tmp2**: Third temperature / PWM tuple.


## FAQ

Can I use the default temperature / PWM tuples?
* Maybe, it depends on your card. The default values are rather
  conservative and should be okay for most cards with 0db mode.
  They may not work properly on cards without 0db mode. 

Okay, were do I get the tuples for my card?
* They can be read from the cards ROM (also known as the BIOS). A good
  starting point is the *Techpowerup Video BIOS Collection*, but not all
  BIOS dumps have the tuples: https://www.techpowerup.com/vgabios/  
  https://github.com/amezin/powerplay-table-editor is able to read the
  fan tables for some GCN generations.
