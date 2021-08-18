package hollow

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// AttributeListParams allow you to filter the results based on attributes
type AttributeListParams struct {
	Namespace        string   `form:"namespace" query:"namespace"`
	Keys             []string `form:"keys" query:"keys"`
	EqualValue       string   `form:"equals" query:"equals"`
	LikeValue        string
	LessThanValue    int `form:"less-than" query:"less-than"`
	GreaterThanValue int `form:"greater-than" query:"greater-than"`
}

func encodeAttributesListParams(alp []AttributeListParams, key string, q url.Values) {
	for _, ap := range alp {
		value := ap.Namespace

		if len(ap.Keys) != 0 {
			value = fmt.Sprintf("%s~%s", value, strings.Join(ap.Keys, "."))

			switch {
			case ap.LessThanValue != 0:
				value = fmt.Sprintf("%s~lt~%d", value, ap.LessThanValue)
			case ap.GreaterThanValue != 0:
				value = fmt.Sprintf("%s~gt~%d", value, ap.GreaterThanValue)
			case ap.LikeValue != "":
				value = fmt.Sprintf("%s~like~%s", value, ap.LikeValue)
			case ap.EqualValue != "":
				value = fmt.Sprintf("%s~eq~%s", value, ap.EqualValue)
			}
		}

		q.Add(key, value)
	}
}

func parseQueryAttributesListParams(c *gin.Context, key string) ([]AttributeListParams, error) {
	var err error

	alp := []AttributeListParams{}

	for _, p := range c.QueryArray(key) {
		// format is "ns~keys.dot.seperated~operation~value"
		parts := strings.Split(p, "~")

		param := AttributeListParams{
			Namespace: parts[0],
		}

		if len(parts) == 1 {
			alp = append(alp, param)
			continue
		}

		param.Keys = strings.Split(parts[1], ".")

		if len(parts) == 4 { //nolint
			switch op := parts[2]; op {
			case "lt":
				param.LessThanValue, err = strconv.Atoi(parts[3])
				if err != nil {
					return nil, err
				}
			case "gt":
				param.GreaterThanValue, err = strconv.Atoi(parts[3])
				if err != nil {
					return nil, err
				}
			case "like":
				param.LikeValue = parts[3]

				// if the like search doesn't contain any % add one at the end
				if !strings.Contains(param.LikeValue, "%") {
					param.LikeValue += "%"
				}
			case "eq":
				param.EqualValue = parts[3]
			}
		}

		alp = append(alp, param)
	}

	return alp, nil
}

func (p *AttributeListParams) queryMods(tblName string) qm.QueryMod {
	nsMod := qm.Where(fmt.Sprintf("%s.namespace = ?", tblName), p.Namespace)

	sqlValues := []interface{}{}
	jsonPath := ""

	// If we only have a namespace and no keys we are limiting by namespace only
	if len(p.Keys) == 0 {
		return nsMod
	}

	for i, k := range p.Keys {
		if i > 0 {
			jsonPath += " , "
		}
		// the actual key is represented as a "?" this helps protect against SQL
		// injection since these strings are passed in by the user.
		jsonPath += "?"

		sqlValues = append(sqlValues, k)
	}

	where := ""

	switch {
	case p.LessThanValue != 0:
		sqlValues = append(sqlValues, p.LessThanValue)
		where = fmt.Sprintf("json_extract_path_text(%s.data::JSON, %s)::int < ?", tblName, jsonPath)
	case p.GreaterThanValue != 0:
		sqlValues = append(sqlValues, p.GreaterThanValue)
		where = fmt.Sprintf("json_extract_path_text(%s.data::JSON, %s)::int > ?", tblName, jsonPath)
	case p.LikeValue != "":
		sqlValues = append(sqlValues, p.LikeValue)
		where = fmt.Sprintf("json_extract_path_text(%s.data::JSONB, %s) LIKE ?", tblName, jsonPath)
	case p.EqualValue != "":
		sqlValues = append(sqlValues, p.EqualValue)
		where = fmt.Sprintf("json_extract_path_text(%s.data::JSONB, %s) = ?", tblName, jsonPath)
	default:
		// we only have keys so we just want to ensure the key is there
		where = fmt.Sprintf("%s.data::JSONB", tblName)

		if len(p.Keys) != 0 {
			for range p.Keys[0 : len(p.Keys)-1] {
				where += " -> ?"
			}

			// query is existing_where ? key
			where += " \\? ?"
		}
	}

	return qm.Expr(
		nsMod,
		qm.And(where, sqlValues...),
	)
}
