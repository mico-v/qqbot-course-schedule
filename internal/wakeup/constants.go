// Package wakeup speaks the WakeUp schedule app's private share-code protocol.
//
// It is a faithful Go port of the Python reference in
// github.com/airline233/WakeUpDecoder (wakeup_share_sim.py): native DES
// signing, RC4 request/response encryption and the /share_schedule/getv2 flow.
// The APK-derived constants below were extracted once from
// WakeUp课程表 6.1.70 and are baked in, so the APK is not needed at runtime.
//
// This package only talks to the network. Turning the decoded share data into
// course events lives in the schedule package.
package wakeup

import "time"

const (
	// Protocol constants recovered from libbaseutil.so. They are native
	// compatibility values: changing any of them makes requests invalid.
	magic    = "8&%d*"
	signAKey = "@fG2SuLA"
	keySalt  = "@#AIjd83#@6B"

	antispamPath = "/pluto/app/antispam"
	sharePath    = "/share_schedule/getv2"

	// DefaultHost is the WakeUp API gateway.
	DefaultHost = "https://api.wakeup.fun"
	// DefaultAndroidID is accepted by the server today but may be banned later.
	DefaultAndroidID = "0000000000000000"
)

// DefaultTimeout bounds each HTTP request.
const DefaultTimeout = 15 * time.Second

// APK facts baked from WakeUp课程表 6.1.70.
const (
	APKPackage      = "com.suda.yzune.wakeupschedule"
	APKVersionCode  = 450
	APKVersionName  = "6.1.70"
	APKChannel      = "100271a"
	APKPublicToken  = "1_XPXQH3c5HRPtFHkSwi3sCCURmT25QfxM"
	APKSignatureMD5 = "318c6d4f74655d4f032fb0466bcfdfbc"
)
