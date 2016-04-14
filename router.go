package main

import (
	"bytes"
	"html/template"
	"net/http"
)

var indexTemp *template.Template

func init() {
	indexTemp = template.Must(template.ParseFiles("index.html"))
}

/**
Request example:
1. ows
http://wfs.geonet.org.nz/geonet/ows?service=WFS&version=1.0.0&request=GetFeature&typeName=geonet:quake_search_v1
&outputFormat=json
&cql_filter=origintime>='2013-06-01'+AND+origintime<'2014-01-01'+AND+usedphasecount>60

2. wms
http://wfs.geonet.org.nz/geonet/wms/kml?layers=geonet:quake_search_v1&maxFeatures=50
*/
func router(w http.ResponseWriter, r *http.Request, b *bytes.Buffer) *result {
	var res *result

	switch {
	case r.URL.Path == "/wms/kml":
		res = getQuakesKml(r, w.Header(), b)
	case r.URL.Path == "/ows":
		res = getQuakesWfs(r, w.Header(), b)
	default: //index page
		indexPage(w)
		res = &statusOK
	}

	return res
}

func indexPage(w http.ResponseWriter) {
	err := indexTemp.Execute(w, nil)
	if err != nil {
		http.Error(http.ResponseWriter(w), err.Error(), http.StatusInternalServerError)
	}
}
