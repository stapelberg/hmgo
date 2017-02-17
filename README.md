## overview

You’re looking at a minimal implementation of a central control unit
for BidCoS-based home automation devices such as the eQ-3 “HomeMatic”
line of products.

The program does not keep any state and does not read any
configuration. Instead, the specific use-case the author needed and
devices the author owns are hard-coded. In a way, you can think of it
as a home automation “configured” in Go, coming with its own low-level
libraries.

## contributions

Note that this project does not accept new features, neither feature
requests nor feature contributions. Documentation or code corrections
on the other hand are very welcome.

The code is published in the hope that it will be useful to someone,
perhaps understanding/implementing the BidCoS protocol themselves :).

If you’d like to see this become an active open source project, please
fork and maintain it, and I’ll gladly add a reference to your version.

## details

This code interacts with the following HomeMatic devices:
* HM-MOD-RPI-PCB (wireless transceiver)
* HM-CC-RT-DN (heating valve drivers)
* HM-TC-IT-WM-W-EU (thermostats)
* HM-ES-PMSw1-Pl (power switch)

All implemented properties of BidCoS events are exposed as
[prometheus](https://prometheus.io/) metrics.

AES encryption is not supported, because I don’t see the need for my
use-case.