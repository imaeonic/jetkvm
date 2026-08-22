//go:build !linux || !arm || !cgo

package native

func panicPlatformNotSupported() {
	panic("platform not supported")
}

func setUpNativeHandlers() {}

func uiInit(rotation uint16) {
	_ = rotation
}

func uiTick() {}

func videoInit(factor float64) error {
	_ = factor
	return nil
}

func videoStart() {}

func videoStop() {}

func uiSetVar(name string, value string) {}

func uiGetVar(name string) string {
	return ""
}

func uiSwitchToScreen(screen string) {}

func uiGetCurrentScreen() string {
	return ""
}

func uiObjAddState(objName string, state string) (bool, error) {
	return false, nil
}

func uiObjClearState(objName string, state string) (bool, error) {
	return false, nil
}

func uiObjAddFlag(objName string, flag string) (bool, error) {
	return false, nil
}

func uiObjClearFlag(objName string, flag string) (bool, error) {
	return false, nil
}

func uiObjHide(objName string) (bool, error) {
	return false, nil
}

func uiObjShow(objName string) (bool, error) {
	return false, nil
}

func uiObjSetOpacity(objName string, opacity int) (bool, error) {
	return false, nil
}

func uiObjFadeIn(objName string, duration uint32) (bool, error) {
	return false, nil
}

func uiObjFadeOut(objName string, duration uint32) (bool, error) {
	return false, nil
}

func uiLabelSetText(objName string, text string) (bool, error) {
	return false, nil
}

func uiImgSetSrc(objName string, src string) (bool, error) {
	return false, nil
}

func uiDispSetRotation(rotation uint16) (bool, error) {
	return false, nil
}

func uiEventCodeToName(code int) string {
	return ""
}

func uiGetLVGLVersion() string {
	return ""
}

func videoGetStreamQualityFactor() (float64, error) {
	return 0, nil
}

func videoSetStreamQualityFactor(factor float64) error {
	return nil
}

func videoSetCodecType(codecType int) error {
	return nil
}

func videoGetCodecType() (int, error) {
	return 0, nil
}

func videoLogStatus() string {
	return ""
}

func videoGetEDID() (string, error) {
	return "", nil
}

func videoSetEDID(edid string) error {
	return nil
}

func videoGetStreamingStatus() VideoStreamingStatus {
	return VideoStreamingStatusInactive
}

func crash() {
	panicPlatformNotSupported()
}
