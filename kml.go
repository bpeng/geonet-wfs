// The kml package is used for rendering Google Earth Keyhole Markup Language
// (KML) files.
package main

import (
	"fmt"
	"math"
	"strings"
)

const (
	ICON_LEGEND_IMG_URL = "http://static.geonet.org.nz/geonet-2.0.4/images/kml/"
	GEONET_LOGO_IMG_URL = "http://static.geonet.org.nz/geonet-2.0.3/images/logo-gns.png"
)

var (
	QUAKE_STYLE_DEPTHS   = [5]string{"0-15", "15-40", "40-100", "100-200", "200-more"}
	QUAKE_DEPTH4COLORS   = [4]int{15, 40, 100, 200}
	KML_ICON_SIZE_SCALES = [7]float64{0.3, 0.5, 0.6, 0.8, 1.0, 1.15, 1.35}
	MAG_CLASSES_KEYS     = [7]string{"2-", "2-3", "3-4", "4-5", "5-6", "6-7", "7-8"}
	MAG_CLASSES_DESC     = [7]string{"Mag 2- earthquakes", "Mag 2 earthquakes", "Mag 3 earthquakes", "Mag 4 earthquakes", "Mag 5 earthquakes", "Mag 6 earthquakes", "Mag 7+ earthquakes"}
)

type renderable interface {
	render() string
}

// KML represents the top-level KML document object.
type KML struct {
	rootFolder *Folder
}

// NewKML returns a pointer to a KML struct.
func NewKML(doc *Folder) *KML {
	return &KML{doc}
}

// Renders the entire KML document.
func (k *KML) Render() string {
	ret := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		`<kml xmlns="http://www.opengis.net/kml/2.2"
		xmlns:atom="http://www.w3.org/2005/Atom"
		xmlns:gx="http://www.google.com/kml/ext/2.2"
		xmlns:ns5="http://www.w3.org/2001/XMLSchema-instance"
		xmlns:xal="urn:oasis:names:tc:ciq:xsdschema:xAL:2.0">` + "\n"

	ret += k.rootFolder.render()

	ret += "</kml>\n"

	return ret
}

// Folder represents a folder in the KML document.
type Folder struct {
	tagName    string
	attributes string
	features   []renderable
}

// Returns a pointer to a new Folder instance.
func NewFolder(name string, attr string) *Folder {
	f := make([]renderable, 0, 10)
	return &Folder{name, attr, f}
}

// Returns a pointer to a new Folder instance.
func NewDocument(name string, open string, desc string) *Folder {
	doc := NewFolder("Document", "")
	doc.AddFeature(NewSimpleContentFolder("name", name))
	doc.AddFeature(NewSimpleContentFolder("open", open))
	doc.AddFeature(NewSimpleContentFolder("description", desc))
	return doc
}

// AddFeature adds a feature (Placemark, another Folder, etc.) to
// the Folder.
func (f *Folder) AddFeature(feature renderable) {
	if feature != nil {
		f.features = append(f.features, feature)
	}
}

func (f *Folder) render() string {
	ret := fmt.Sprintf("<%s %s>", f.tagName, f.attributes)
	for _, feature := range f.features {
		ret += feature.render()
	}
	ret += fmt.Sprintf("</%s>", f.tagName)
	return ret
}

//simple folder without children
type SimpleFolder struct {
	TagName    string
	Attributes string
}

func NewSimpleFolder(tag string, attr string) *SimpleFolder {
	return &SimpleFolder{tag, attr}
}

func (sf *SimpleFolder) render() string {
	ret := fmt.Sprintf("<%s %s/>\n", sf.TagName, sf.Attributes)
	return ret
}

//simple folder with only string content
type SimpleContentFolder struct {
	TagName string
	content string
}

func NewSimpleContentFolder(tag string, cont string) *SimpleContentFolder {
	return &SimpleContentFolder{tag, cont}
}

func (scf *SimpleContentFolder) render() string {
	return fmt.Sprintf("<%s>%s</%s>\n", scf.TagName, scf.content, scf.TagName)
}

// Style represents a style used for a geometry object (point, line,
// polygon, etc.)
type Style struct {
	id         string
	iconStyle  *IconStyle
	labelStyle *LabelStyle
}

func NewStyle(id string, is *IconStyle, ls *LabelStyle) *Style {
	return &Style{id, is, ls}
}

func (s *Style) render() string {
	var ret string
	if s.id != "" {
		ret = fmt.Sprintf("<Style id=\"%s\">\n", s.id)
	} else {
		ret = fmt.Sprintf("<Style>\n")
	}
	if s.iconStyle != nil {
		ret += s.iconStyle.render()
	}
	if s.labelStyle != nil {
		ret += s.labelStyle.render()
	}
	ret += "</Style>\n"
	return ret
}

type LabelStyle struct {
	scale float64
}

func NewLabelStyle(sc float64) *LabelStyle {
	return &LabelStyle{sc}
}

func (lbl *LabelStyle) render() string {
	ret := fmt.Sprintf("<LabelStyle><scale>%.1f</scale></LabelStyle>\n", lbl.scale)
	return ret
}

type IconStyle struct {
	iconScale float64
	iconHead  float64
	icon      *Icon
	hotspot   *SimpleFolder
}

func NewIconStyle(is float64, ih float64) *IconStyle {
	return &IconStyle{iconScale: is, iconHead: ih}
}

func (s *IconStyle) setIcon(ic *Icon) {
	s.icon = ic
}

func (s *IconStyle) setHotspot(ht *SimpleFolder) {
	s.hotspot = ht
}

func (s *IconStyle) render() string {
	ret := fmt.Sprintf(
		"<IconStyle>\n" +
			fmt.Sprintf("<scale>%.1f</scale>\n", s.iconScale) +
			fmt.Sprintf("<heading>%.1f</heading>\n", s.iconHead))

	if s.icon != nil {
		ret += s.icon.render()
	}
	if s.hotspot != nil {
		ret += s.hotspot.render()
	}
	ret += "</IconStyle>\n"

	return ret
}

type Icon struct {
	href            string
	refreshInterval float64
	viewRefreshTime float64
	viewBoundScale  float64
}

func NewIcon(url string, rf float64, vf float64, vb float64) *Icon {
	return &Icon{url, rf, vf, vb}
}

func (ic *Icon) render() string {
	ret := fmt.Sprintf("<Icon><href>%s</href>", ic.href) +
		fmt.Sprintf("<refreshInterval>%.1f</refreshInterval>\n", ic.refreshInterval) +
		fmt.Sprintf("<viewBoundScale>%.1f</viewBoundScale>\n", ic.viewBoundScale) +
		fmt.Sprintf("<viewRefreshTime>%.1f</viewRefreshTime>\n", ic.viewRefreshTime) +
		"</Icon>\n"
	return ret
}

type StyleMap struct {
	id    string
	pairs []renderable
}

func NewStyleMap(name string) *StyleMap {
	p := make([]renderable, 0, 10)
	return &StyleMap{name, p}
}

func (st *StyleMap) AddPair(pair renderable) {
	if pair != nil {
		st.pairs = append(st.pairs, pair)
	}
}

func (st *StyleMap) render() string {
	ret := fmt.Sprintf("<StyleMap id=\"%s\">", st.id)
	for _, pair := range st.pairs {
		ret += pair.render()
	}
	ret += "</StyleMap>\n"
	return ret
}

type Pair struct {
	key string
	url string
}

func NewPair(id1 string, url1 string) *Pair {
	return &Pair{id1, url1}
}

func (p *Pair) render() string {
	return fmt.Sprintf("<Pair><key>%s</key><styleUrl>%s</styleUrl></Pair>", p.key, p.url)
}

// Point represents a point on the Earth
type Point struct {
	Lat float64 // latitude
	Lon float64 // longitude
}

// NewPoint returns a pointer to a new Point instance.  Invalid points (those
// that with lat outside of +/-90.0 and lon outside of +/-180.0, NaN or Inf
// will return nil.
func NewPoint(lat float64, lon float64) *Point {
	if math.IsNaN(lat) || math.IsInf(lat, 0) {
		return nil
	}

	if lat > 90.0 || lat < -90.0 {
		return nil
	}

	if math.IsNaN(lon) || math.IsInf(lon, 0) {
		return nil
	}

	if lon > 180.0 || lon < -180.0 {
		return nil
	}

	return &Point{lat, lon}
}

func (p *Point) render() string {
	ret := "<Point>\n" +
		fmt.Sprintf("<coordinates>%g,%g</coordinates>\n", p.Lon, p.Lat) +
		"</Point>\n"
	return ret
}

// Placemark represents a placemark in the KML document.  All geometry
// objects (points, lines, polygons, etc.) must be within a Placemark
// instance.
type Placemark struct {
	name         string
	timeStamp    string
	geometry     renderable
	styleUrl     string
	style        *Style
	extendedData renderable
}

type ExtendedData struct {
	data []renderable
}

func NewExtendedData() *ExtendedData {
	nd := make([]renderable, 0, 10)
	return &ExtendedData{nd}
}

func (ex *ExtendedData) AddData(newdata renderable) {
	if newdata != nil {
		ex.data = append(ex.data, newdata)
	}
}

func (ex *ExtendedData) render() string {
	ret := ""
	if len(ex.data) > 0 {
		ret = "<ExtendedData>\n"
		for _, dt := range ex.data {
			ret += dt.render()
		}
		ret += "</ExtendedData>\n"
	}
	return ret
}

type Data struct {
	name  string
	value string
}

func NewData(n string, v string) *Data {
	return &Data{n, v}
}

func (dt *Data) render() string {
	ret := "<Data name=\"" + dt.name + "\">\n" +
		fmt.Sprintf("<value>%s</value>\n", dt.value)
	ret += "</Data>\n"
	return ret
}

// NewPlacemark returns a pointer to a new Placemark instance.  It takes a
// name, description, and a geometry object (Point, Polygon, etc.) as
// parameters.
func NewPlacemark(nm string, tmsp string, geom renderable) *Placemark {
	return &Placemark{name: nm, timeStamp: tmsp, geometry: geom}
}

// SetStyle sets the style of the Placemark to the specified name.  The KML
// document must have a Style instance with a matching name (see NewStyle).
func (pm *Placemark) SetStyleUrl(name string) {
	name = strings.TrimSpace(name)
	if len(name) > 0 {
		pm.styleUrl = name
	}
}

func (pm *Placemark) SetStyle(stl *Style) {
	pm.style = stl
}

func (pm *Placemark) SetExtendedData(data renderable) {
	pm.extendedData = data
}

func (pm *Placemark) render() string {
	ret := "<Placemark>\n" +
		fmt.Sprintf("<name>%s</name>\n", pm.name)
	if len(pm.timeStamp) > 0 {
		ret += fmt.Sprintf("<TimeStamp><when>%s</when></TimeStamp>\n", pm.timeStamp)
	}
	if len(pm.styleUrl) > 0 {
		ret += fmt.Sprintf("<styleUrl>%s</styleUrl>\n", pm.styleUrl)
	}
	ret += pm.style.render()
	ret += pm.geometry.render()
	ret += pm.extendedData.render()
	ret += "</Placemark>\n"

	return ret
}

//###
func getKmlStyleUrl(dep float64) string {
	for index, depth := range QUAKE_DEPTH4COLORS {
		if dep < float64(depth) {
			return "#" + QUAKE_STYLE_DEPTHS[index]
		}
	}
	return "#" + QUAKE_STYLE_DEPTHS[len(QUAKE_DEPTH4COLORS)]
}

func getQuakeMagClassIndex(mag float64) int {
	index := 0
	if mag < 2 {
		index = 0
	} else if mag >= 7 {
		index = 6
	} else {
		index = int(mag) - 1
	}
	return index
}

func getKmlIconSize(magnitude float64) float64 {
	index := getQuakeMagClassIndex(magnitude)
	return KML_ICON_SIZE_SCALES[index]
}

func getQuakeMagClass(magnitude float64) [2]string {
	index := getQuakeMagClassIndex(magnitude)
	return [2]string{MAG_CLASSES_KEYS[index], MAG_CLASSES_DESC[index]}
}

func createKmlStyle(id string, iconPath string, scale float64) *Style {
	icon1 := NewIcon((ICON_LEGEND_IMG_URL + "depth-" + iconPath + ".png"), 0.0, 0.0, 0.0)
	iconstyle1 := NewIconStyle(0.0, 0.0)
	iconstyle1.setIcon(icon1)
	hot := NewSimpleFolder("hotSpot", `x="0.5" y="0.5" xunits="fraction" yunits="fraction"`)
	iconstyle1.setHotspot(hot)

	lblStyle := NewLabelStyle(scale)

	return NewStyle(id, iconstyle1, lblStyle)
}

//create screen overlays for GNS logo and legend
func createGnsKmlScreenOverlays() *Folder {
	screenOverLays := NewFolder("Folder", "")
	screenOverLays.AddFeature(NewSimpleContentFolder("name", "Screen overlays"))
	screenOverLays.AddFeature(NewSimpleContentFolder("open", "1"))

	//2.1 GeoNet Legend
	legenIcon := NewIcon((ICON_LEGEND_IMG_URL + "legend-depth.png"), 0.0, 0.0, 0.0)
	legendOverLay := NewFolder("ScreenOverlay", "")
	legendOverLay.AddFeature(NewSimpleContentFolder("drawOrder", "0"))
	legendOverLay.AddFeature(legenIcon)
	legendOverLay.AddFeature(NewSimpleFolder("overlayXY", `x="0.0" y="0.0" xunits="pixels" yunits="pixels"`))
	legendOverLay.AddFeature(NewSimpleFolder("screenXY", ` x="10.0" y="20.0" xunits="pixels" yunits="pixels"`))
	legendOverLay.AddFeature(NewSimpleFolder("rotationXY", `x="0.0" y="0.0" xunits="pixels" yunits="pixels"`))
	legendOverLay.AddFeature(NewSimpleContentFolder("rotation", "0.0"))

	screenOverLays.AddFeature(legendOverLay)

	//<FolderSimpleExtensionGroup ns5:type="ScreenOverlayType">
	legendOverLayExtensionGroup := NewFolder("FolderSimpleExtensionGroup", `ns5:type="ScreenOverlayType"`)
	legendOverLayExtensionGroup.AddFeature(NewSimpleContentFolder("drawOrder", "0"))
	legendOverLayExtensionGroup.AddFeature(legenIcon)
	legendOverLayExtensionGroup.AddFeature(NewSimpleFolder("overlayXY", `x="0.0" y="0.0" xunits="pixels" yunits="pixels"`))
	legendOverLayExtensionGroup.AddFeature(NewSimpleFolder("screenXY", `x="10.0" y="20.0" xunits="pixels" yunits="pixels"`))
	legendOverLayExtensionGroup.AddFeature(NewSimpleFolder("rotationXY", `x="0.0" y="0.0" xunits="pixels" yunits="pixels"`))
	legendOverLayExtensionGroup.AddFeature(NewSimpleContentFolder("rotation", "0.0"))
	//add feature
	screenOverLays.AddFeature(legendOverLayExtensionGroup)

	//2.2 GeoNet Logo
	logoIcon := NewIcon(GEONET_LOGO_IMG_URL, 0.0, 0.0, 0.0)
	logoOverLay := NewFolder("ScreenOverlay", "")
	logoOverLay.AddFeature(NewSimpleContentFolder("drawOrder", "0"))
	logoOverLay.AddFeature(logoIcon)
	logoOverLay.AddFeature(NewSimpleFolder("screenXY", `x="0.9" y="0.2" xunits="fraction" yunits="fraction"`))
	logoOverLay.AddFeature(NewSimpleContentFolder("rotation", "0.0"))

	//add feature
	screenOverLays.AddFeature(logoOverLay)

	//<FolderSimpleExtensionGroup ns5:type="ScreenOverlayType">
	logoOverLayExtensionGroup := NewFolder("FolderSimpleExtensionGroup", `ns5:type="ScreenOverlayType"`)
	logoOverLayExtensionGroup.AddFeature(NewSimpleContentFolder("drawOrder", "0"))
	logoOverLayExtensionGroup.AddFeature(logoIcon)
	logoOverLayExtensionGroup.AddFeature(NewSimpleFolder("screenXY", `x="0.9" y="0.2" xunits="fraction" yunits="fraction"`))
	logoOverLayExtensionGroup.AddFeature(NewSimpleContentFolder("rotation", "0.0"))

	//add feature
	screenOverLays.AddFeature(logoOverLayExtensionGroup)

	return screenOverLays
}
