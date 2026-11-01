// Package clock is the agent's source of "now" for anything that decides when
// prayers happen. In a normal build Now is exactly time.Now. Builds made with the
// "azanfaketime" tag (used only by the end-to-end tests) can start the clock at a
// chosen instant via AZAN_FAKE_NOW, so a whole process can be run on a DST day.
package clock

import "time"

// Now returns the current time.
var Now = time.Now
