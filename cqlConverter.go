package main

import (
	"bytes"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

/**
 * A cql converter that converts CQL query to SQL query
 * Baishan 15/4/2016
 */
const (
	DATE_PATTERN      = "(\\d{4})-(\\d{1,2})-(\\d{1,2})"
	DATE_TIME_PATTERN = "(\\d{4})-(\\d{1,2})-(\\d{1,2})T(\\d{1,2}):(\\d{1,2}):(\\d{1,2})"
	//token types
	TT_EOF          = -1
	TT_WORD         = -3
	TT_STRING       = 999  // quoted string
	TT_SORTBY       = 1008 // The "sortby" operator
	TT_PARENTHESIS  = 1100 //()
	TT_LOGIC_OPS    = 1101 //and or
	TT_RELATION_OPS = 1102 //>, <, =, >=, <=, !=, ==
	TT_TIMESTRING   = 1103 //2016-4-12T22:00:00
	TT_BBOX         = 1104 //BBOX(origin_geom,174,-41,175,-42)
	TT_WITHIN       = 1105 //WITHIN(origin_geom,POLYGON((172.951 -41.767, 172.001 -42.832, 169.564 -44.341,+172.312 -45.412, 175.748 -42.908, 172.951 -41.767)))
	TT_DWITHIN      = 1106 //DWITHIN(origin_geom,Point+(175+-41),0.5,feet)
)

type CqlConverter struct {
	cqlCharArray     []rune //char array of the CQL string
	cqlLen           int    //length of the CQL string
	currentIndex     int    //current index in the cqlCharArray
	currentTokenType int    //current token type in the CQL string
	currentToken     string //current token in the CQL
	BBOX             string
}

//search string contains another string
func SearchStringInString(s string, ch string) bool {
	return (strings.Index(s, ch)) >= 0
}

/**
 * convenient pattern match (treat error as false)
 */
func PatternMatch(pattern string, str string) bool {
	match, err := regexp.MatchString(pattern, str)
	if err != nil {
		return false
	}
	return match
}

// NewCqlConverterL returns a pointer to a CqlConverter struct.
func NewCqlConverter(cql string) *CqlConverter {
	cqlConverter := CqlConverter{}
	cqlConverter.cqlCharArray = []rune(cql)
	cqlConverter.cqlLen = len(cqlConverter.cqlCharArray)
	return &cqlConverter
}

func (cql *CqlConverter) Render() string {
	var ret string
	switch cql.currentTokenType {
	case TT_BBOX:
		ret = "bboc"
	case TT_WITHIN:
		ret = "within"
	case TT_LOGIC_OPS:
		ret = "logic"
	case TT_PARENTHESIS:
		ret = "paranthesis"
	case TT_RELATION_OPS:
		ret = "relation"
	case TT_DWITHIN:
		ret = "dwithin"
	case TT_TIMESTRING:
		ret = "time"
	case TT_WORD:
		ret = "word"
	case TT_STRING:
		ret = "string"
	case TT_EOF:
		ret = "END"
	default:
		ret = ""
	}
	ret += ">>" + cql.currentToken
	return ret
}

/**
 * move to next token, update token and type
 * token separators: "()/<>= \t\r\n"
 */
func (cql *CqlConverter) NextToken() {
	//eat whitespace
	for cql.currentIndex < cql.cqlLen && SearchStringInString(" \t\r\n", string(cql.cqlCharArray[cql.currentIndex])) {
		cql.currentIndex++
	}

	//eof
	if cql.currentIndex == cql.cqlLen {
		cql.currentTokenType = TT_EOF
		cql.currentToken = ""
		return
	}
	//get current char
	c := string(cql.cqlCharArray[cql.currentIndex])
	var buf bytes.Buffer
	var lval string
	//1. separators
	if SearchStringInString("()", c) {
		cql.currentTokenType = TT_PARENTHESIS
		cql.currentToken = c
		cql.currentIndex++
		//2. comparitor
	} else if SearchStringInString("<>=!", c) {
		cql.currentIndex++
		//two-char comparitor
		twoCharOps := false

		//log.Println("2. relation c " + c)

		if cql.currentIndex < cql.cqlLen {
			d := string(cql.cqlCharArray[cql.currentIndex])
			//log.Println("2. relation d " + d)
			if SearchStringInString("<>=", d) {
				cql.currentToken = c + d
				cql.currentTokenType = TT_RELATION_OPS
				twoCharOps = true
				cql.currentIndex++
			}
		}
		if !twoCharOps { //single char ops
			if SearchStringInString("<>=", c) {
				cql.currentToken = c
				cql.currentTokenType = TT_RELATION_OPS
			} else { //only the ! char
				cql.currentIndex--
			}
		}
		//3. quoted string
	} else if SearchStringInString("\"", c) { //no single-quotes
		cql.currentTokenType = TT_STRING
		//remember quote char
		mark := c
		cql.currentIndex++
		escaped := false

		buf.Reset() //reset buffer
		for cql.currentIndex < cql.cqlLen {
			if !escaped && (string(cql.cqlCharArray[cql.currentIndex]) == mark) { //terminator
				break
			}
			if escaped && SearchStringInString("*?^\\", string(cql.cqlCharArray[cql.currentIndex])) { //no escaping for d-quote
				buf.WriteString("\\")
			}
			if !escaped && string(cql.cqlCharArray[cql.currentIndex]) == "\\" { //escape-char
				escaped = true
				cql.currentIndex++
				continue
			}
			escaped = false //reset escape
			buf.WriteString(string(cql.cqlCharArray[cql.currentIndex]))
			cql.currentIndex++
		}
		cql.currentToken = buf.String()
		lval = strings.ToLower(cql.currentToken)
		if cql.currentIndex < cql.cqlLen {
			cql.currentIndex++
		} else { //unterminated
			cql.currentTokenType = TT_EOF //notify error
		}
	} else { //4. WORD, including: logic ops, bbox, within, dwithin, timestring
		cql.currentTokenType = TT_WORD
		buf.Reset() //reset buffer
		for cql.currentIndex < cql.cqlLen && !SearchStringInString("()/<>= \t\r\n", string(cql.cqlCharArray[cql.currentIndex])) {
			buf.WriteString(string(cql.cqlCharArray[cql.currentIndex]))
			cql.currentIndex++
		}
		cql.currentToken = buf.String()
		lval = strings.ToLower(cql.currentToken)
		//strip leading and tailing single quotes
		stripVal := strings.TrimLeft(cql.currentToken, "'")
		stripVal = strings.TrimRight(stripVal, "'")

		if lval == "or" || lval == "and" || lval == "not" {
			cql.currentTokenType = TT_LOGIC_OPS
		} else if lval == "bbox" {
			cql.currentTokenType = TT_BBOX
		} else if lval == "within" {
			cql.currentTokenType = TT_WITHIN
		} else if lval == "dwithin" {
			cql.currentTokenType = TT_DWITHIN
		} else if lval == "sortby" {
			cql.currentTokenType = TT_SORTBY
		} else if PatternMatch(DATE_PATTERN, stripVal) || PatternMatch(DATE_TIME_PATTERN, stripVal) {
			cql.currentTokenType = TT_TIMESTRING
		}
	}
}

/**
  *sql:
  *  BBOX(origin_geom,174,-41,175,-42)
  sql:
  ST_Contains(ST_SetSRID(ST_Envelope('LINESTRING(174 -41,175 -42)'::geometry),4326), origin_geom)
  * @param sql
*/
func (cql *CqlConverter) ToBBoxSql(sql *bytes.Buffer) error {
	token := cql.currentToken //bbox
	//log.Println("token1 " + token)
	cql.NextToken()

	if cql.currentTokenType != TT_PARENTHESIS {
		//log.Fatalln("Error, not valid bbox!!")
		return errors.New("not valid bbox!!")
	}
	tokens := make([]string, 0)

	for ")" != cql.currentToken { //move until the close parenthesis
		cql.NextToken()
		token = cql.currentToken // origin_geom,174,-41,175,-42
		//log.Println("token2 " + token)
		if ")" != token {
			for _, value := range strings.Split(token, ",") {
				if value != "" {
					tokens = append(tokens, value)
				}
			}
		}
	}

	//log.Println("## tokens", len(tokens))

	if cql.currentTokenType != TT_PARENTHESIS {
		//log.Fatal("Error, not valid bbox!!")
		return errors.New("not valid bbox!!")
	}
	//String[] bboxContents = token3.split("[, ]+");
	if len(tokens) == 5 {
		sql.WriteString("ST_Contains(ST_SetSRID(ST_Envelope('LINESTRING(")
		sql.WriteString(tokens[1])
		sql.WriteString(" ")
		sql.WriteString(tokens[2])
		sql.WriteString(",")
		sql.WriteString(tokens[3])
		sql.WriteString(" ")
		sql.WriteString(tokens[4])
		sql.WriteString(")'::geometry),4326),")
		sql.WriteString(tokens[0])
		sql.WriteString(")")
		//keep the bbox string for later user
		cql.BBOX = tokens[1] + "," + tokens[2] + "," + tokens[3] + "," + tokens[4]
	}
	return nil
}

/**
* get comma separated tokens between parenthesis
* (origin_geom,POLYGON((172.951 -41.767,172.001 -42.832,169.564 -44.341,172.312 -45.412,175.748 -42.908,172.951 -41.767)))
* (origin_geom,Point(175 -41),500,meters)
 */
func (cql *CqlConverter) GetParenthesisedTokens() []string {
	//get all tokens for the WITHIN claus, parse all comma separated tokens
	var token string
	openParenthesis := 1 //already at open parenthesis
	closeParenthesis := 0
	tokens := make([]string, 0)
	for true {
		cql.NextToken()
		token = cql.currentToken // origin_geom,174,-41,175,-42
		//log.Println("token2 " + token)

		for _, value := range strings.Split(token, ",") {
			if value != "" {
				tokens = append(tokens, value)
			}
		}

		if "(" == token {
			openParenthesis++
		}
		if ")" == token {
			closeParenthesis++
		}

		//fmt.Printf("openParenthesis %d closeParenthesis %d", openParenthesis, closeParenthesis)
		if closeParenthesis > 0 && openParenthesis == closeParenthesis {
			break
		}
	}
	return tokens
}

/**
  * cql:
  WITHIN(origin_geom,POLYGON((172.951 -41.767,172.001 -42.832,169.564 -44.341,172.312 -45.412,175.748 -42.908,172.951 -41.767)))
  sql:
  ST_Within(origin_geom, ST_GeomFromText('POLYGON((172.951 -41.767,172.001 -42.832,169.564 -44.341,172.312 -45.412,175.748 -42.908,172.951 -41.767))',4326))

  * @param sql
*/
func (cql *CqlConverter) ToWithinSql(sql *bytes.Buffer) error {
	token := cql.currentToken //within
	//log.Println("token1 " + token)
	cql.NextToken()

	if "(" != cql.currentToken {
		//log.Fatalln("Error, not valid WITHIN!!")
		return errors.New("not valid WITHIN!!")
	}

	//get all tokens for the WITHIN claus, parse all comma separated tokens
	tokens := cql.GetParenthesisedTokens()
	//log.Println((tokens))

	sql.WriteString("ST_Within(")
	sql.WriteString(tokens[0])
	sql.WriteString(", ST_GeomFromText('")

	openParenthesis := 0
	closeParenthesis := 0
	i := 1 //start from 1
	numeric := 0
	//get the polygon contents
	for i < len(tokens) {
		token = tokens[i]
		i++
		if "(" == token {
			openParenthesis++
		}
		if ")" == token {
			closeParenthesis++
		}

		if "POLYGON" == strings.ToUpper(token) {
			sql.WriteString("POLYGON((")
			numeric = 0
		}
		//get the coordinates
		if openParenthesis == 2 && closeParenthesis == 0 {
			if PatternMatch("-?[.\\d]+", token) { //numeric
				numeric++
				if numeric > 1 {
					if numeric%2 == 0 {
						sql.WriteString(" ")
					} else {
						sql.WriteString(",")
					}
				}
				sql.WriteString(token)
			}
		}
		//fmt.Printf("openParenthesis %d closeParenthesis %d", openParenthesis, closeParenthesis)
		if openParenthesis == closeParenthesis && closeParenthesis > 0 {
			break
		}
	}
	sql.WriteString("))', 4326))")
	return nil
}

/**
 * cql:
 * DWITHIN(origin_geom,Point(175 -41),500,meters)
 * sql:
 * ST_DWithin(origin_geom::Geography, ST_GeomFromText('POINT(172.951 -41.767)', 4326)::Geography, 5000)
 *
 * !!!WE ONLY DO METERS!!
 * @param sql
 */
func (cql *CqlConverter) ToDWithinSql(sql *bytes.Buffer) error {
	token := cql.currentToken //within
	//log.Println("token1 " + token)
	cql.NextToken()

	if "(" != cql.currentToken {
		//log.Fatalln("Error, not valid DWITHIN!!")
		return errors.New("not valid DWITHIN!!")
	}

	//get all tokens for the WITHIN claus, parse all comma separated tokens
	tokens := cql.GetParenthesisedTokens()
	//log.Println((tokens))

	sql.WriteString("ST_DWithin(")
	sql.WriteString(tokens[0])
	sql.WriteString("::Geography, ST_GeomFromText('")

	openParenthesis := 0
	closeParenthesis := 0
	i := 1
	numeric := 0
	numParenthesis := 0
	geom := ""
	//get the geometry contents
	for i < len(tokens) {
		token = tokens[i]

		//log.Println(token)
		i++
		if "(" == token {
			openParenthesis++
		}
		if ")" == token {
			closeParenthesis++
		}
		//log.Println("token3 " + token)
		if "POLYGON" == strings.ToUpper(token) || "POLYLINE" == strings.ToUpper(token) || "POINT" == strings.ToUpper(token) {
			geom = token
			if "POLYGON" == strings.ToUpper(token) || "POLYLINE" == strings.ToUpper(token) {
				sql.WriteString(token)
				sql.WriteString("((")
				numParenthesis = 2
			} else if "POINT" == strings.ToUpper(token) {
				sql.WriteString("POINT(")
				numParenthesis = 1
			}
			numeric = 0
		}
		//get the coordinates
		if openParenthesis == numParenthesis && closeParenthesis == 0 {
			if PatternMatch("-?[.\\d]+", token) { //numeric
				numeric++
				if numeric > 1 {
					if numeric%2 == 0 {
						sql.WriteString(" ")
					} else {
						sql.WriteString(",")
					}
				}
				sql.WriteString(token)
			}
		}
		//fmt.Printf("openParenthesis %d closeParenthesis %d", openParenthesis, closeParenthesis)
		if openParenthesis == closeParenthesis && closeParenthesis > 0 {
			break
		}
	}
	if "POLYGON" == strings.ToUpper(geom) || "POLYLINE" == strings.ToUpper(geom) {
		sql.WriteString("))', 4326)::Geography")
	} else if "POINT" == strings.ToUpper(geom) {
		sql.WriteString(")', 4326)::Geography")
	} else {
		return errors.New("No valid geometry for function DWITHIN!!")
	}
	//get the distance
	if i < len(tokens) {
		token = tokens[i]
		//log.Println("## distance " + token)
		if PatternMatch("[.\\d]+", token) {
			distance, err := strconv.ParseFloat(token, 64)
			if err != nil {
				//log.Fatalln("error parsing distance!!")
				distance = -1
				return errors.New("error parsing distance!!")
			}
			i++
			if i < len(tokens) { //get unit
				token = strings.ToLower(tokens[i])
				//log.Println("## unit " + token)
				if "meters" == token {
				} else if "feet" == token {
					//log.Println("## change feets to meeters " + token)
					distance = 0.3048 * distance
				} else {
					//log.Fatalln("wrong unit for distance, should be in [meters, feet]!!")
					return errors.New("wrong unit for distance, should be in [meters, feet]!!")
				}
			}
			sql.WriteString(", ")
			sql.WriteString(strconv.FormatFloat(distance, 'f', -1, 64))
		}
	}
	sql.WriteString(")")

	return nil
}

/**
*   convert cql to sql
 */
func (cql *CqlConverter) ToSQL() (string, error) {
	var (
		sql bytes.Buffer
		err error
	)
	err = nil
	//go through the CQL string and convert
	for cql.currentTokenType != TT_EOF {
		cql.NextToken()
		//1. leading space
		if cql.currentTokenType == TT_RELATION_OPS || cql.currentTokenType == TT_LOGIC_OPS {
			sql.WriteString(" ")
		}
		//2. do content
		if cql.currentTokenType == TT_TIMESTRING {
			sql.WriteString(cql.currentToken)
			sql.WriteString("::timestamptz") //DB column type
		} else if cql.currentTokenType == TT_RELATION_OPS && cql.currentToken == "==" { //convert to =
			sql.WriteString("=")
		} else if cql.currentTokenType == TT_BBOX {
			err = cql.ToBBoxSql(&sql)
		} else if cql.currentTokenType == TT_WITHIN {
			err = cql.ToWithinSql(&sql)
		} else if cql.currentTokenType == TT_DWITHIN {
			err = cql.ToDWithinSql(&sql)
		} else { //just the token
			sql.WriteString(cql.currentToken)
		}
		//3. end space
		if cql.currentTokenType == TT_RELATION_OPS || cql.currentTokenType == TT_LOGIC_OPS {
			sql.WriteString(" ")
		}
		//log.Println(cql.Render())
	}
	return sql.String(), err

}
