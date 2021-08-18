package hollow

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// OperatorType is used to control what kind of search is performed for an AttributeListParams value.
type OperatorType string

const (
	// TypeEqual means the value has to match the keys exactly
	TypeEqual OperatorType = "eq"
	// TypeLike allows you to pass in a value with % in it and match anything like it. If your string has no % in it one will be added to the end automatically
	TypeLike = "like"
	// TypeGreaterThan will convert the value at the given key to an int and return results that are greater than Value
	TypeGreaterThan = "gt"
	// TypeLessThan will convert the value at the given key to an int and return results that are less than Value
	TypeLessThan = "lt"
)

// AttributeListParams allow you to filter the results based on attributes
type AttributeListParams struct {
	Namespace string
	Keys      []string
	Operator  OperatorType
	Value     string
}

func encodeAttributesListParams(alp []AttributeListParams, key string, q url.Values) {
	for _, ap := range alp {
		value := ap.Namespace

		if len(ap.Keys) != 0 {
			value = fmt.Sprintf("%s~%s", value, strings.Join(ap.Keys, "."))

			if ap.Operator != "" {
				value = fmt.Sprintf("%s~%s~%s", value, ap.Operator, ap.Value)
			}
		}

		q.Add(key, value)
	}
}

func parseQueryAttributesListParams(c *gin.Context, key string) []AttributeListParams {
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

		if len(parts) == 4 { // nolint

			switch o := (*OperatorType)(&parts[2]); *o {
			case TypeEqual, TypeLike, TypeGreaterThan, TypeLessThan:
				param.Operator = *o
				param.Value = parts[3]
			}

			// if the like search doesn't contain any % add one at the end
			if param.Operator == TypeLike && !strings.Contains(param.Value, "%") {
				param.Value += "%"
			}
		}

		alp = append(alp, param)
	}

	return alp
}

// queryMods converts the list params into sql conditions that can be added to
// sql queries
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

	switch p.Operator {
	case TypeLessThan:
		sqlValues = append(sqlValues, p.Value)
		where = fmt.Sprintf("json_extract_path_text(%s.data::JSONB, %s)::int < ?", tblName, jsonPath)
	case TypeGreaterThan:
		sqlValues = append(sqlValues, p.Value)
		where = fmt.Sprintf("json_extract_path_text(%s.data::JSONB, %s)::int > ?", tblName, jsonPath)
	case TypeLike:
		sqlValues = append(sqlValues, p.Value)
		where = fmt.Sprintf("json_extract_path_text(%s.data::JSONB, %s) LIKE ?", tblName, jsonPath)
	case TypeEqual:
		sqlValues = append(sqlValues, p.Value)
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
