# geonet-wfs

Provide WFS service for the GeoNet Earthquake Catalog, replace the current GeoServer as required by https://github.com/GeoNet/tickets/issues/434

## Query examples:
as specified in http://info.geonet.org.nz/display/appdata/Advanced+Queries

## Development
Convert CQL query to SQL and get quakes from the GeoNet quakes database, and output in: CSV, GML, GeoJSON, KML format

Support simple CQL queries as specified in http://info.geonet.org.nz/display/appdata/Advanced+Queries