package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	empty_param_value   = -1000
	GML_BBOX_NZ         = "164,-49 -176, -32"
	GML3_BOUND_LOWER_NZ = "164 -49"
	GML3_BOUND_UPPER_NZ = "-176 -32"
	GEONET_ASSET_URL    = "http://static.geonet.org.nz/"

	RFC3339_FORMAT      = "2006-01-02T15:04:05.999Z"
	UTC_KML_TIME_FORMAT = "January 02 2006 at 3:04:05 pm"
	NZ_KML_TIME_FORMAT  = "Monday, 02 January 2006 at 3:04:05 pm"

	CONTENT_TYPE_XML = "application/xml"
	CONTENT_TYPE_KML = "application/vnd.google-earth.kml+xml"

	// These constants represent part of a public API and can't be changed.
	V1GeoJSON = "application/vnd.geo+json;version=1"
	V1JSON    = "application/json;version=1"
	V1CSV     = "text/csv;version=1"
	V2GeoJSON = "application/vnd.geo+json;version=2"
	V2JSON    = "application/json;version=2"
	V2CSV     = "text/csv;version=2"
)

var (
	NZTzLocation   *time.Location
	requiredParams = []string{"outputFormat"}
	optionalParams = []string{"service",
		"version",
		"request",
		"typeName", //for wfs
		"layers",   //for kml
		"maxFeatures",
		"cql_filter",
		"subtype",
	}
)

func init() {
	//get NZ time zone location
	l, e := time.LoadLocation("NZ")
	if e == nil {
		NZTzLocation = l
	} else {
		NZTzLocation = time.Local
		log.Println("Unable to get NZ timezone, use local time instead!")
	}
}

func getQuakesWfs(r *http.Request, h http.Header, b *bytes.Buffer) *result {
	//1. check query parameters
	if res := checkQuery(r, requiredParams, optionalParams); !res.ok {
		return res
	}
	v := r.URL.Query()
	params := getQueryParams(v)
	log.Println("##outputFormat|", params.outputFormat, "| sub type", params.subType)
	if params.outputFormat == "JSON" {
		return getQuakesGeoJson(r, h, b, params)
	} else if params.outputFormat == "CSV" {
		return getQuakesCsv(r, h, b, params)
	} else if params.outputFormat == "GML2" {
		return getQuakesGml2(r, h, b, params)
		//text/xml; subtype=gml/3.2
	} else if params.outputFormat == "TEXT/XML" && params.subType == "GML/3.2" {
		log.Println("##2 outputFormat", params.outputFormat, " sub type", params.subType)
		return getQuakesGml3(r, h, b, params)
	}
	return &statusOK
}

/**
* ideally to use go kml library, but they are too basic, without screen overlay and style map
* so use string content instead.
* kml?region=canterbury&startdate=2010-6-29T21:00:00&enddate=2015-7-29T22:00:00
 */
func getQuakesKml(r *http.Request, h http.Header, b *bytes.Buffer) *result {
	//1. check query parameters
	if res := checkQuery(r, []string{}, optionalParams); !res.ok {
		return res
	}

	v := r.URL.Query()
	sqlPre := `select publicid, eventtype, to_char(origintime, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') AS origintime,
     latitude, longitude, depth, depthtype, magnitude, magnitudetype, evaluationmethod, evaluationstatus,
     evaluationmode, earthmodel, usedphasecount,usedstationcount, minimumdistance, azimuthalgap, magnitudeuncertainty,
     originerror, magnitudestationcount, to_char(modificationtime, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') AS modificationtime
     from wfs.quake_search_v1 `

	params := getQueryParams(v)

	sqlString, err1 := getSqlQueryString(sqlPre, params)
	if err1 != nil {
		return badRequest(err1.Error())
	}

	rows, err := db.Query(sqlString)

	if err != nil {
		return internalServerError(err)
	}
	defer rows.Close()
	count := 0

	allQuakeFolders := make(map[string]*Folder)

	for rows.Next() { //21 fields
		var ( //note the null values
			publicid              string
			origintime            string
			modificationtime      sql.NullString
			eventtype             sql.NullString
			latitude              float64
			longitude             float64
			depth                 sql.NullFloat64
			depthtype             sql.NullString
			magnitude             sql.NullFloat64
			magnitudetype         sql.NullString
			evaluationmethod      sql.NullString
			evaluationstatus      sql.NullString
			evaluationmode        sql.NullString
			earthmodel            sql.NullString
			usedphasecount        sql.NullInt64
			usedstationcount      sql.NullInt64
			minimumdistance       sql.NullFloat64
			azimuthalgap          sql.NullFloat64
			magnitudeuncertainty  sql.NullFloat64
			originerror           sql.NullFloat64
			magnitudestationcount sql.NullInt64
		)

		err := rows.Scan(&publicid, &eventtype, &origintime, &latitude, &longitude, &depth, &depthtype,
			&magnitude, &magnitudetype, &evaluationmethod, &evaluationstatus,
			&evaluationmode, &earthmodel, &usedphasecount, &usedstationcount,
			&minimumdistance, &azimuthalgap, &magnitudeuncertainty, &originerror, &magnitudestationcount,
			&modificationtime,
		)
		if err != nil {
			return internalServerError(err)
		}
		count++

		mag := 0.0
		if magnitude.Valid {
			mag = magnitude.Float64
		}
		dep := 0.0
		if depth.Valid {
			dep = depth.Float64
		}

		iconSt := NewIconStyle(getKmlIconSize(mag), 0.0)
		style := NewStyle("", iconSt, nil)
		quakePm := NewPlacemark("quake."+publicid, origintime, NewPoint(latitude, longitude))
		quakePm.SetStyleUrl(getKmlStyleUrl(dep))
		quakePm.SetStyle(style)

		exData := NewExtendedData()
		exData.AddData(NewData("Public Id", publicid))

		t, err := time.Parse(RFC3339_FORMAT, origintime)
		if err != nil {
			log.Panic("time format error", err)
			return internalServerError(err)
		}

		tu := t.In(time.UTC)
		utcTime := tu.Format(UTC_KML_TIME_FORMAT)
		exData.AddData(NewData("Universal Time", utcTime))

		tnz := t.In(NZTzLocation)
		nzTime := tnz.Format(NZ_KML_TIME_FORMAT)

		exData.AddData(NewData("NZ Standard Time", nzTime))

		if depth.Valid {
			exData.AddData(NewData("Focal Depth (km)", fmt.Sprintf("%g", depth.Float64)))
		}

		if magnitude.Valid {
			exData.AddData(NewData("Magnitude", fmt.Sprintf("%g", magnitude.Float64)))
		}

		if magnitudetype.Valid {
			exData.AddData(NewData("Magnitude Type", magnitudetype.String))
		}

		if depthtype.Valid {
			exData.AddData(NewData("Depth Type", depthtype.String))
		}

		if evaluationmethod.Valid {
			exData.AddData(NewData("Evaluation Method", evaluationmethod.String))
		}

		if evaluationstatus.Valid {
			exData.AddData(NewData("Evaluation Status", evaluationstatus.String))
		}

		if evaluationmode.Valid {
			exData.AddData(NewData("Evaluation Mode", evaluationmode.String))
		}

		if earthmodel.Valid {
			exData.AddData(NewData("Earth Model", earthmodel.String))
		}

		if usedphasecount.Valid {
			exData.AddData(NewData("Used Face Count", fmt.Sprintf("%d", usedphasecount.Int64)))
		}

		if usedstationcount.Valid {
			exData.AddData(NewData("Used station Count", fmt.Sprintf("%d", usedstationcount.Int64)))
		}

		if magnitudestationcount.Valid {
			exData.AddData(NewData("Magnitude station Count", fmt.Sprintf("%d", magnitudestationcount.Int64)))
		}

		if minimumdistance.Valid {
			exData.AddData(NewData("Minimum Distance", fmt.Sprintf("%g", minimumdistance.Float64)))
		}

		if azimuthalgap.Valid {
			exData.AddData(NewData("Azimuthal Gap", fmt.Sprintf("%g", azimuthalgap.Float64)))
		}

		if originerror.Valid {
			exData.AddData(NewData("Origin Error", fmt.Sprintf("%g", originerror.Float64)))
		}

		if magnitudeuncertainty.Valid {
			exData.AddData(NewData("Magnitude Uncertainty", fmt.Sprintf("%g", magnitudeuncertainty.Float64)))
		}

		quakePm.SetExtendedData(exData)
		if magnitude.Valid {
			quakeMagClass := getQuakeMagClass(magnitude.Float64)
			quakeFolder := allQuakeFolders[quakeMagClass[0]]

			if quakeFolder == nil {
				quakeFolder = NewFolder("Folder", "")
				quakeFolder.AddFeature(NewSimpleContentFolder("name", quakeMagClass[1]))
			}

			quakeFolder.AddFeature(quakePm)
			allQuakeFolders[quakeMagClass[0]] = quakeFolder
		}

	}

	rows.Close()

	doc := NewDocument(fmt.Sprintf("%d New Zealand Earthquakes", count), "1",
		"New Zealand earthquake as located by the GeoNet project.")
	//1. add style map and style
	for _, depth := range QUAKE_STYLE_DEPTHS {
		styleMap := NewStyleMap(depth)
		pair1 := NewPair("normal", "#inactive-"+depth)
		pair2 := NewPair("highlight", "#active-"+depth)
		styleMap.AddPair(pair1)
		styleMap.AddPair(pair2)
		doc.AddFeature(styleMap)
		doc.AddFeature(createKmlStyle("active-"+depth, depth, 1.0))
		doc.AddFeature(createKmlStyle("inactive-"+depth, depth, 0.0))
	}

	//2. add screen overlays
	screenOverLays := createGnsKmlScreenOverlays()
	//add to doc
	doc.AddFeature(screenOverLays)

	//3. add quakes folder
	for i := len(MAG_CLASSES_KEYS) - 1; i >= 0; i-- {
		folder := allQuakeFolders[MAG_CLASSES_KEYS[i]]
		if folder != nil {
			doc.AddFeature(folder)
		}
	}

	kml := NewKML(doc)
	b.WriteString(kml.Render())

	//w.Header().Set("Content-Type", "application/xml") //test!!
	h.Set("Content-Type", CONTENT_TYPE_KML)
	h.Set("Content-Disposition", `attachment; filename="earthquakes.kml"`)
	return &statusOK

}

/**
* GML3 format
**/
func getQuakesGml3(r *http.Request, h http.Header, b *bytes.Buffer, params *QueryParams) *result {
	sqlPre := `select publicid, eventtype, to_char(origintime, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') AS origintime,
           latitude, longitude, depth, depthtype, magnitude,  magnitudetype, evaluationmethod, evaluationstatus,
           evaluationmode, earthmodel, usedphasecount,usedstationcount, minimumdistance, azimuthalgap, magnitudeuncertainty,
           originerror, magnitudestationcount, to_char(modificationtime, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') AS modificationtime,
           ST_AsGML(3,origin_geom) as gml from wfs.quake_search_v1 `

	sqlString, err1 := getSqlQueryString(sqlPre, params)
	if err1 != nil {
		return badRequest(err1.Error())
	}

	rows, err := db.Query(sqlString)

	if err != nil {
		return internalServerError(err)
	}
	defer rows.Close()

	// var b bytes.Buffer
	eol := []byte("\n")

	var (
		boundLower string
		boundUpper string
	)
	if params.bbox != "" {
		bboxarray := BBox2Array(params.bbox)
		if len(bboxarray) == 4 {
			boundLower = bboxarray[0] + " " + bboxarray[1]
			boundUpper = bboxarray[2] + " " + bboxarray[3]
		}
	}

	if boundLower == "" {
		boundLower = GML3_BOUND_LOWER_NZ
	}
	if boundUpper == "" {
		boundUpper = GML3_BOUND_UPPER_NZ
	}

	t := time.Now()

	b.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
    <wfs:FeatureCollection
       xmlns:wfs="http://www.opengis.net/wfs/2.0"
       xmlns:gml="http://www.opengis.net/gml/3.2"
       xmlns:geonet="http://geonet.org.nz"
       xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
       timeStamp="` + t.Format(RFC3339_FORMAT) + `" ` +
		`xsi:schemaLocation="http://geonet.org.nz http://wfs.geonet.org.nz/geonet/quakes
       http://www.opengis.net/gml/3.2 http://wfs.geonet.org.nz/schemas/gml/3.2.1/gml.xsd
       http://www.opengis.net/wfs/2.0 http://wfs.geonet.org.nz/schemas/wfs/2.0/wfs.xsd">
     <wfs:boundedBy>
        <gml:Envelope srsDimension="2" srsName="http://www.opengis.net/gml/srs/epsg.xml#4326">
          <gml:lowerCorner>` + boundLower + `</gml:lowerCorner>
           <gml:upperCorner>` + boundUpper + `</gml:upperCorner>
       </gml:Envelope>
     </wfs:boundedBy>`))

	b.Write(eol)

	for rows.Next() {
		var ( //note the null values
			publicid              string
			origintime            string
			modificationtime      sql.NullString
			eventtype             sql.NullString
			latitude              float64
			longitude             float64
			depth                 sql.NullFloat64
			depthtype             sql.NullString
			magnitude             sql.NullFloat64
			magnitudetype         sql.NullString
			evaluationmethod      sql.NullString
			evaluationstatus      sql.NullString
			evaluationmode        sql.NullString
			earthmodel            sql.NullString
			usedphasecount        sql.NullInt64
			usedstationcount      sql.NullInt64
			minimumdistance       sql.NullFloat64
			azimuthalgap          sql.NullFloat64
			magnitudeuncertainty  sql.NullFloat64
			originerror           sql.NullFloat64
			magnitudestationcount sql.NullInt64
			gml                   string
		)

		err := rows.Scan(&publicid, &eventtype, &origintime, &latitude, &longitude, &depth, &depthtype,
			&magnitude, &magnitudetype, &evaluationmethod, &evaluationstatus,
			&evaluationmode, &earthmodel, &usedphasecount, &usedstationcount,
			&minimumdistance, &azimuthalgap, &magnitudeuncertainty, &originerror, &magnitudestationcount,
			&modificationtime, &gml,
		)
		if err != nil {
			return internalServerError(err)
		}
		b.Write([]byte("<wfs:member>\n"))
		b.Write([]byte(fmt.Sprintf("<geonet:quake gml:id=\"quake.%s\">\n", publicid)))
		//
		b.Write([]byte("<gml:boundedBy>\n<gml:Envelope srsDimension=\"2\" srsName=\"http://www.opengis.net/gml/srs/epsg.xml#4326\">\n"))
		b.Write([]byte(fmt.Sprintf("<gml:lowerCorner>%g %g</gml:lowerCorner>\n", longitude, latitude)))
		b.Write([]byte(fmt.Sprintf("<gml:upperCorner>%g %g</gml:upperCorner>\n", longitude, latitude)))
		b.Write([]byte("</gml:Envelope>\n</gml:boundedBy>\n"))

		b.Write([]byte(fmt.Sprintf("<geonet:publicid>%s</geonet:publicid>\n", publicid)))
		b.Write([]byte(fmt.Sprintf("<geonet:origintime>%s</geonet:origintime>\n", origintime)))
		b.Write([]byte(fmt.Sprintf("<geonet:latitude>%g</geonet:latitude>\n", latitude)))
		b.Write([]byte(fmt.Sprintf("<geonet:longitude>%g</geonet:longitude>\n", longitude)))
		if eventtype.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:eventtype>%s</geonet:eventtype>\n", eventtype.String)))
		}
		if modificationtime.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:modificationtime>%s</geonet:modificationtime>\n", modificationtime.String)))
		}
		if depth.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:depth>%g</geonet:depth>\n", depth.Float64)))
		}
		if depthtype.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:depthtype>%s</geonet:depthtype>\n", depthtype.String)))
		}
		if magnitude.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:magnitude>%g</geonet:magnitude>\n", magnitude.Float64)))
		}
		if magnitudetype.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:magnitudetype>%s</geonet:magnitudetype>\n", magnitudetype.String)))
		}
		if evaluationmethod.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:evaluationmethod>%s</geonet:evaluationmethod>\n", evaluationmethod.String)))
		}
		if evaluationstatus.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:evaluationstatus>%s</geonet:evaluationstatus>\n", evaluationstatus.String)))
		}
		if evaluationmode.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:evaluationmode>%s</geonet:evaluationmode>\n", evaluationmode.String)))
		}
		if earthmodel.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:earthmodel>%s</geonet:earthmodel>\n", earthmodel.String)))
		}
		if usedphasecount.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:usedphasecount>%d</geonet:usedphasecount>\n", usedphasecount.Int64)))
		}
		if usedstationcount.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:usedstationcount>%d</geonet:usedstationcount>\n", usedstationcount.Int64)))
		}
		if minimumdistance.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:minimumdistance>%g</geonet:minimumdistance>\n", minimumdistance.Float64)))
		}
		if azimuthalgap.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:azimuthalgap>%g</geonet:azimuthalgap>\n", azimuthalgap.Float64)))
		}
		if magnitudeuncertainty.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:magnitudeuncertainty>%g</geonet:magnitudeuncertainty>\n", magnitudeuncertainty.Float64)))
		}
		if originerror.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:originerror>%g</geonet:originerror>\n", originerror.Float64)))
		}
		if magnitudestationcount.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:magnitudestationcount>%d</geonet:magnitudestationcount>\n", magnitudestationcount.Int64)))
		}
		//geonet:origin_geom
		b.Write([]byte(fmt.Sprintf("<geonet:origin_geom>%s</geonet:origin_geom>\n", gml)))
		b.Write([]byte("</geonet:quake></wfs:member>\n"))
	}

	rows.Close()
	b.Write([]byte(`</wfs:FeatureCollection>`))

	// send result response
	h.Set("Content-Type", CONTENT_TYPE_XML)
	return &statusOK
}

func getQuakesGml2(r *http.Request, h http.Header, b *bytes.Buffer, params *QueryParams) *result {
	sqlPre := `select publicid, eventtype, to_char(origintime, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') AS origintime,
           latitude, longitude, depth, depthtype, magnitude,  magnitudetype, evaluationmethod, evaluationstatus,
           evaluationmode, earthmodel, usedphasecount,usedstationcount, minimumdistance, azimuthalgap, magnitudeuncertainty,
           originerror, magnitudestationcount, to_char(modificationtime, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') AS modificationtime,
           ST_AsGML(origin_geom) as gml from wfs.quake_search_v1 `

	sqlString, err1 := getSqlQueryString(sqlPre, params)
	if err1 != nil {
		return badRequest(err1.Error())
	}

	rows, err := db.Query(sqlString)

	if err != nil {
		return internalServerError(err)
	}
	defer rows.Close()

	// var b bytes.Buffer
	eol := []byte("\n")
	bbox1 := getGmlBbox(params.bbox)

	if bbox1 == "" {
		bbox1 = GML_BBOX_NZ
	}
	b.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
    <wfs:FeatureCollection xmlns:wfs="http://www.opengis.net/wfs"
     xmlns:gml="http://www.opengis.net/gml"
     xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
     xmlns:geonet="http://geonet.org.nz"
     xsi:schemaLocation="http://geonet.org.nz http://geonet.org.nz/quakes http://www.opengis.net/wfs http://schemas.opengis.net/wfs/1.0.0/WFS-basic.xsd">
     <gml:boundedBy>
       <gml:Box srsName="http://www.opengis.net/gml/srs/epsg.xml#4326">
          <gml:coordinates decimal="." cs="," ts=" ">` + bbox1 + `</gml:coordinates>
       </gml:Box>
     </gml:boundedBy>`))
	b.Write(eol)

	for rows.Next() {
		var ( //note the null values
			publicid              string
			origintime            string
			modificationtime      sql.NullString
			eventtype             sql.NullString
			latitude              float64
			longitude             float64
			depth                 sql.NullFloat64
			depthtype             sql.NullString
			magnitude             sql.NullFloat64
			magnitudetype         sql.NullString
			evaluationmethod      sql.NullString
			evaluationstatus      sql.NullString
			evaluationmode        sql.NullString
			earthmodel            sql.NullString
			usedphasecount        sql.NullInt64
			usedstationcount      sql.NullInt64
			minimumdistance       sql.NullFloat64
			azimuthalgap          sql.NullFloat64
			magnitudeuncertainty  sql.NullFloat64
			originerror           sql.NullFloat64
			magnitudestationcount sql.NullInt64
			gml                   string
		)

		err := rows.Scan(&publicid, &eventtype, &origintime, &latitude, &longitude, &depth, &depthtype,
			&magnitude, &magnitudetype, &evaluationmethod, &evaluationstatus,
			&evaluationmode, &earthmodel, &usedphasecount, &usedstationcount,
			&minimumdistance, &azimuthalgap, &magnitudeuncertainty, &originerror, &magnitudestationcount,
			&modificationtime, &gml,
		)
		if err != nil {
			return internalServerError(err)
		}
		b.Write([]byte("<gml:featureMember>\n"))
		b.Write([]byte(fmt.Sprintf("<geonet:quake fid=\"quake.%s\">\n", publicid)))
		//
		b.Write([]byte("<gml:boundedBy>\n<gml:Box srsName=\"http://www.opengis.net/gml/srs/epsg.xml#4326\">\n"))
		b.Write([]byte(fmt.Sprintf("<gml:coordinates decimal=\".\" cs=\",\" ts=\" \">%g,%g %g,%g</gml:coordinates>\n", longitude, latitude, longitude, latitude)))
		b.Write([]byte("</gml:Box>\n</gml:boundedBy>\n"))

		b.Write([]byte(fmt.Sprintf("<geonet:publicid>%s</geonet:publicid>\n", publicid)))
		b.Write([]byte(fmt.Sprintf("<geonet:origintime>%s</geonet:origintime>\n", origintime)))
		b.Write([]byte(fmt.Sprintf("<geonet:latitude>%g</geonet:latitude>\n", latitude)))
		b.Write([]byte(fmt.Sprintf("<geonet:longitude>%g</geonet:longitude>\n", longitude)))
		if eventtype.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:eventtype>%s</geonet:eventtype>\n", eventtype.String)))
		}
		if modificationtime.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:modificationtime>%s</geonet:modificationtime>\n", modificationtime.String)))
		}
		if depth.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:depth>%g</geonet:depth>\n", depth.Float64)))
		}
		if depthtype.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:depthtype>%s</geonet:depthtype>\n", depthtype.String)))
		}
		if magnitude.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:magnitude>%g</geonet:magnitude>\n", magnitude.Float64)))
		}
		if magnitudetype.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:magnitudetype>%s</geonet:magnitudetype>\n", magnitudetype.String)))
		}
		if evaluationmethod.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:evaluationmethod>%s</geonet:evaluationmethod>\n", evaluationmethod.String)))
		}
		if evaluationstatus.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:evaluationstatus>%s</geonet:evaluationstatus>\n", evaluationstatus.String)))
		}
		if evaluationmode.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:evaluationmode>%s</geonet:evaluationmode>\n", evaluationmode.String)))
		}
		if earthmodel.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:earthmodel>%s</geonet:earthmodel>\n", earthmodel.String)))
		}
		if usedphasecount.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:usedphasecount>%d</geonet:usedphasecount>\n", usedphasecount.Int64)))
		}
		if usedstationcount.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:usedstationcount>%d</geonet:usedstationcount>\n", usedstationcount.Int64)))
		}
		if minimumdistance.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:minimumdistance>%g</geonet:minimumdistance>\n", minimumdistance.Float64)))
		}
		if azimuthalgap.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:azimuthalgap>%g</geonet:azimuthalgap>\n", azimuthalgap.Float64)))
		}
		if magnitudeuncertainty.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:magnitudeuncertainty>%g</geonet:magnitudeuncertainty>\n", magnitudeuncertainty.Float64)))
		}
		if originerror.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:originerror>%g</geonet:originerror>\n", originerror.Float64)))
		}
		if magnitudestationcount.Valid {
			b.Write([]byte(fmt.Sprintf("<geonet:magnitudestationcount>%d</geonet:magnitudestationcount>\n", magnitudestationcount.Int64)))
		}
		//geonet:origin_geom
		b.Write([]byte(fmt.Sprintf("<geonet:origin_geom>%s</geonet:origin_geom>\n", gml)))
		b.Write([]byte("</geonet:quake></gml:featureMember>\n"))
	}

	rows.Close()
	b.Write([]byte(`</wfs:FeatureCollection>`))

	// send result response
	h.Set("Content-Type", CONTENT_TYPE_XML)
	return &statusOK
}

func getQuakesCsv(r *http.Request, h http.Header, b *bytes.Buffer, params *QueryParams) *result {
	//21  fields
	sqlPre := `select format('%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s',
               publicid,eventtype,to_char(origintime, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
               to_char(modificationtime, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),longitude, latitude, magnitude,
               depth,magnitudetype, depthtype, evaluationmethod, evaluationstatus, evaluationmode, earthmodel, usedphasecount,
               usedstationcount,magnitudestationcount, minimumdistance,
               azimuthalgap,originerror,magnitudeuncertainty) as csv from wfs.quake_search_v1`

	sqlString, err1 := getSqlQueryString(sqlPre, params)
	if err1 != nil {
		return badRequest(err1.Error())
	}

	rows, err := db.Query(sqlString)

	if err != nil {
		return internalServerError(err)
	}
	defer rows.Close()
	defer rows.Close()

	var (
		// b bytes.Buffer
		d string
	)
	eol := []byte("\n")

	b.Write([]byte("publicid,eventtype,origintime,modificationtime,longitude, latitude, magnitude, depth,magnitudetype,depthtype," +
		"evaluationmethod,evaluationstatus,evaluationmode,earthmodel,usedphasecount,usedstationcount,magnitudestationcount,minimumdistance," +
		"azimuthalgap,originerror,magnitudeuncertainty"))
	b.Write(eol)
	for rows.Next() {
		err := rows.Scan(&d)
		if err != nil {
			return internalServerError(err)
		}
		b.Write([]byte(d))
		b.Write(eol)
	}
	rows.Close()

	// send result response
	h.Set("Content-Disposition", `attachment; filename="earthquakes.csv"`)
	h.Set("Content-Type", V1CSV)
	return &statusOK
}

//http://hutl14681.gns.cri.nz:8081/geojson?limit=100&bbox=163.60840,-49.18170,182.98828,-32.28713&startdate=2015-6-27T22:00:00&enddate=2015-7-27T23:00:00
//(r *http.Request, h http.Header, b *bytes.Buffer) *result
func getQuakesGeoJson(r *http.Request, h http.Header, b *bytes.Buffer, params *QueryParams) *result {
	sqlPre := `select publicid, eventtype, to_char(origintime, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') AS origintime,
              depth, depthtype, magnitude, magnitudetype, evaluationmethod, evaluationstatus,
              evaluationmode, earthmodel, usedphasecount,usedstationcount, minimumdistance, azimuthalgap, magnitudeuncertainty,
              originerror, magnitudestationcount, to_char(modificationtime, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') AS modificationtime,
              ST_AsGeoJSON(origin_geom) as geojson from wfs.quake_search_v1`

	sqlString, err1 := getSqlQueryString(sqlPre, params)
	if err1 != nil {
		return badRequest(err1.Error())
	}

	rows, err := db.Query(sqlString)

	if err != nil {
		return internalServerError(err)
	}
	defer rows.Close()
	allFeatures := make([]Feature, 0)
	//
	for rows.Next() {
		var ( //note the null values
			publicid              string
			origintime            string
			modificationtime      sql.NullString
			eventtype             sql.NullString
			depth                 sql.NullFloat64
			depthtype             sql.NullString
			magnitude             sql.NullFloat64
			magnitudetype         sql.NullString
			evaluationmethod      sql.NullString
			evaluationstatus      sql.NullString
			evaluationmode        sql.NullString
			earthmodel            sql.NullString
			usedphasecount        sql.NullInt64
			usedstationcount      sql.NullInt64
			minimumdistance       sql.NullFloat64
			azimuthalgap          sql.NullFloat64
			magnitudeuncertainty  sql.NullFloat64
			originerror           sql.NullFloat64
			magnitudestationcount sql.NullInt64
			geojson               string
		)

		err := rows.Scan(&publicid, &eventtype, &origintime, &depth, &depthtype,
			&magnitude, &magnitudetype, &evaluationmethod, &evaluationstatus,
			&evaluationmode, &earthmodel, &usedphasecount, &usedstationcount,
			&minimumdistance, &azimuthalgap, &magnitudeuncertainty, &originerror, &magnitudestationcount,
			&modificationtime, &geojson,
		)
		if err != nil {
			return internalServerError(err)
		}
		quakeFeature := Feature{Type: "Feature"}
		//get geometry
		var featureGeo FeatureGeometry
		err = json.Unmarshal([]byte(geojson), &featureGeo)
		if err != nil {
			log.Panic("error", err)
			return internalServerError(err)
		}
		quakeFeature.Geometry = featureGeo
		//get properties
		quakeProp := QuakeProperties{Publicid: publicid,
			Origintime: origintime,
		}
		//only get the non null values
		if eventtype.Valid {
			quakeProp.Eventtype = eventtype.String
		}
		if modificationtime.Valid {
			quakeProp.Modificationtime = modificationtime.String
		}
		if depth.Valid {
			quakeProp.Depth = depth.Float64
		}
		if depthtype.Valid {
			quakeProp.Depthtype = depthtype.String
		}
		if magnitude.Valid {
			quakeProp.Magnitude = magnitude.Float64
		}
		if magnitudetype.Valid {
			quakeProp.Magnitudetype = magnitudetype.String
		}
		if evaluationmethod.Valid {
			quakeProp.Evaluationmethod = evaluationmethod.String
		}
		if evaluationstatus.Valid {
			quakeProp.Evaluationstatus = evaluationstatus.String
		}
		if evaluationmode.Valid {
			quakeProp.Evaluationmode = evaluationmode.String
		}
		if earthmodel.Valid {
			quakeProp.Earthmodel = earthmodel.String
		}
		if usedphasecount.Valid {
			quakeProp.Usedphasecount = usedphasecount.Int64
		}
		if usedstationcount.Valid {
			quakeProp.Usedstationcount = usedstationcount.Int64
		}
		if minimumdistance.Valid {
			quakeProp.Minimumdistance = minimumdistance.Float64
		}
		if azimuthalgap.Valid {
			quakeProp.Azimuthalgap = azimuthalgap.Float64
		}
		if magnitudeuncertainty.Valid {
			quakeProp.Magnitudeuncertainty = magnitudeuncertainty.Float64
		}
		if originerror.Valid {
			quakeProp.Originerror = originerror.Float64
		}
		if magnitudestationcount.Valid {
			quakeProp.Magnitudestationcount = magnitudestationcount.Int64
		}

		quakeFeature.Properties = quakeProp
		allFeatures = append(allFeatures, quakeFeature)
	}
	rows.Close()

	outputJson := GeoJsonFeatureCollection{
		Type:     "FeatureCollection",
		Features: allFeatures,
	}

	// send result response
	h.Set("Content-Type", V1GeoJSON)
	// h.Set("Accept", V1GeoJSON)
	jsonBytes, err := json.Marshal(outputJson)
	if err != nil {
		return internalServerError(err)
	}

	b.Write(jsonBytes)

	return &statusOK
}

func getQueryParams(v url.Values) *QueryParams {
	return &QueryParams{
		outputFormat: strings.ToUpper(v.Get("outputFormat")),
		maxFeatures:  parseIntVal(v.Get("maxFeatures")),
		cqlFilter:    v.Get("cql_filter"),
		subType:      strings.ToUpper(v.Get("subtype")),
	}
}

func parseIntVal(valstring string) int {
	if valstring != "" {
		if val, err := strconv.Atoi(valstring); err == nil {
			return val
		}
	}
	return empty_param_value
}

/* generate sql query string based on query parameters from url*/
func getSqlQueryString(sqlPre string, params *QueryParams) (string, error) {
	sql := sqlPre
	if params.cqlFilter != "" {
		cql := NewCqlConverter(params.cqlFilter)
		cql2Sql, err := cql.ToSQL()
		params.bbox = cql.BBOX
		if err == nil {
			sql += fmt.Sprintf(" WHERE %s", cql2Sql)
		} else {
			return "", err
		}
	}

	if params.maxFeatures != empty_param_value {
		sql += fmt.Sprintf(" limit %d", params.maxFeatures)
	}
	log.Println("##sql", sql)
	return sql, nil

}

func BBox2Array(bbox string) []string {
	bboxarray := strings.Split(bbox, ",")
	//remove empty
	for i := len(bboxarray) - 1; i >= 0; i-- {
		val := strings.TrimSpace(bboxarray[i])
		// Condition to decide if current element has to be deleted:
		if val == "" {
			bboxarray = append(bboxarray[:i], bboxarray[i+1:]...)
		}
	}
	return bboxarray
}

func getGmlBbox(bbox string) string {
	if bbox != "" {
		bboxarray := BBox2Array(bbox)
		if len(bboxarray) == 4 {
			return bboxarray[0] + "," + bboxarray[1] + " " + bboxarray[2] + "," + bboxarray[3]
		}
	}
	return ""
}

type GeoJsonFeatureCollection struct {
	Type     string    `json:"type"`
	Features []Feature `json:"features"`
}

type Feature struct {
	Type       string          `json:"type"`
	Geometry   FeatureGeometry `json:"geometry"`
	Properties QuakeProperties `json:"properties"`
}

type QuakeProperties struct {
	Publicid              string  `json:"publicid"`
	Eventtype             string  `json:"eventtype,omitempty"`
	Origintime            string  `json:"origintime"`
	Modificationtime      string  `json:"modificationtime,omitempty"`
	Depth                 float64 `json:"depth"`
	Depthtype             string  `json:"depthtype,omitempty"`
	Magnitude             float64 `json:"magnitude,omitempty"`
	Magnitudetype         string  `json:"magnitudetype,omitempty"`
	Evaluationmethod      string  `json:"evaluationmethod,omitempty"`
	Evaluationstatus      string  `json:"evaluationstatus,omitempty"`
	Evaluationmode        string  `json:"evaluationmode,omitempty"`
	Earthmodel            string  `json:"earthmodel,omitempty"`
	Usedphasecount        int64   `json:"usedphasecount,omitempty"`
	Usedstationcount      int64   `json:"usedstationcount,omitempty"`
	Minimumdistance       float64 `json:"minimumdistance,omitempty"`
	Azimuthalgap          float64 `json:"azimuthalgap,omitempty"`
	Magnitudeuncertainty  float64 `json:"magnitudeuncertainty,omitempty"`
	Originerror           float64 `json:"originerror,omitempty"`
	Magnitudestationcount int64   `json:"magnitudestationcount,omitempty"`
}

type FeatureGeometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

type QueryParams struct {
	outputFormat string
	subType      string //sub type of outputFormat
	maxFeatures  int
	cqlFilter    string
	bbox         string
}
