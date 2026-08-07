package settingsui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// radarTheme approximates the web app's dark navy/blue look (internal/templates' DaisyUI dark
// theme) so the native settings window doesn't feel like a different product - a color palette
// only, not a pixel-perfect port: Fyne's theme system controls colors/fonts/sizes globally, not
// per-widget borders/roundedness/icons the way the web app's Tailwind/DaisyUI CSS does.
type radarTheme struct{}

var _ fyne.Theme = (*radarTheme)(nil)

var (
	radarBackground = color.NRGBA{R: 0x0b, G: 0x11, B: 0x20, A: 0xff}
	radarPanel      = color.NRGBA{R: 0x13, G: 0x1b, B: 0x2e, A: 0xff}
	radarInput      = color.NRGBA{R: 0x11, G: 0x18, B: 0x27, A: 0xff}
	radarBorder     = color.NRGBA{R: 0x1e, G: 0x29, B: 0x3b, A: 0xff}
	radarHover      = color.NRGBA{R: 0x1a, G: 0x24, B: 0x3a, A: 0xff}
	radarPrimary    = color.NRGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}
	radarForeground = color.NRGBA{R: 0xe2, G: 0xe8, B: 0xf0, A: 0xff}
	radarMuted      = color.NRGBA{R: 0x64, G: 0x74, B: 0x8b, A: 0xff}
	radarSuccess    = color.NRGBA{R: 0x22, G: 0xc5, B: 0x5e, A: 0xff}
)

func (radarTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return radarBackground
	case theme.ColorNamePrimary:
		return radarPrimary
	case theme.ColorNameButton, theme.ColorNameHeaderBackground:
		return radarPanel
	case theme.ColorNameInputBackground:
		return radarInput
	case theme.ColorNameInputBorder, theme.ColorNameSeparator:
		return radarBorder
	case theme.ColorNameForeground:
		return radarForeground
	case theme.ColorNameDisabled, theme.ColorNamePlaceHolder:
		return radarMuted
	case theme.ColorNameHover, theme.ColorNamePressed:
		return radarHover
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0x55}
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return radarPanel
	case theme.ColorNameScrollBar:
		return radarBorder
	case theme.ColorNameSuccess:
		return radarSuccess
	case theme.ColorNameForegroundOnPrimary:
		return color.White
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (radarTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (radarTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (radarTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
