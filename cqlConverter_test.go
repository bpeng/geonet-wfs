package main

import (
	"bytes"
	"testing"
)

/**
 * test the cqlConverter
 */
func TestGetParenthesisedTokens(t *testing.T) {
	cqlString := `(origin_geom, 174,-41,175, -42)`
	cql := NewCqlConverter(cqlString)
	cql.NextToken()
	tokens := cql.GetParenthesisedTokens()
	//t.Log(tokens)
	if len(tokens) != 6 {
		t.Fail()
	}
}

func TestToBBoxSql(t *testing.T) {
	cqlString := `BBOX(origin_geom, 174,-41,175, -42)`
	expected := `ST_Contains(ST_SetSRID(ST_Envelope('LINESTRING(174 -41,175 -42)'::geometry),4326),origin_geom)`
	var sql bytes.Buffer
	cql := NewCqlConverter(cqlString)
	cql.NextToken()
	err := cql.ToBBoxSql(&sql)
	//t.Log(sql.String())

	if err != nil || sql.String() != expected {
		t.Fail()
	}
}

func TestToWithinSql(t *testing.T) {
	cqlString := `WITHIN(origin_geom,POLYGON((172.951 -41.767,172.001 -42.832,169.564 -44.341,172.312 -45.412,175.748 -42.908,172.951 -41.767)))`
	expected := `ST_Within(origin_geom, ST_GeomFromText('POLYGON((172.951 -41.767,172.001 -42.832,169.564 -44.341,172.312 -45.412,175.748 -42.908,172.951 -41.767))', 4326))`
	var sql bytes.Buffer
	cql := NewCqlConverter(cqlString)
	cql.NextToken()
	err := cql.ToWithinSql(&sql)
	t.Log(sql.String())

	if err != nil || sql.String() != expected {
		t.Fail()
	}
}

func TestToDWithinSql(t *testing.T) {
	cqlString := `DWITHIN(origin_geom,Point(172.951 -41.767),5000,meters)`
	expected := `ST_DWithin(origin_geom::Geography, ST_GeomFromText('POINT(172.951 -41.767)', 4326)::Geography, 5000)`
	var sql bytes.Buffer
	cql := NewCqlConverter(cqlString)
	cql.NextToken()
	err := cql.ToDWithinSql(&sql)
	t.Log(sql.String())

	if err != nil || sql.String() != expected {
		t.Fail()
	}
}

func TestToSql(t *testing.T) {
	cqlString := `(origintime>='2013-06-01' AND origintime<'2016-04-12T22:00:00') or usedphasecount != 60`
	expected := `(origintime >= '2013-06-01'::timestamptz AND origintime < '2016-04-12T22:00:00'::timestamptz) or usedphasecount != 60`
	cql := NewCqlConverter(cqlString)
	sql, err := cql.ToSQL()
	//t.Log(sql)
	if err != nil || sql != expected {
		t.Fail()
	}
}
