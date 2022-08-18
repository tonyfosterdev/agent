package flow

import (
	"reflect"
	"strings"

	"github.com/grafana/agent/component"
	"github.com/grafana/agent/pkg/river/value"

	"github.com/grafana/agent/pkg/flow/rivertypes"

	"github.com/grafana/agent/pkg/river/rivertags"
)

// Field should probably spit this into block and non block fields
type Field struct {
	ID           string      `json:"id,omitempty"`
	Key          string      `json:"key,omitempty"`
	Name         string      `json:"name,omitempty"`
	Label        string      `json:"label,omitempty"`
	Type         string      `json:"type,omitempty"`
	References   []string    `json:"references,omitempty"`
	ReferencedBy []string    `json:"reference_by,omitempty"`
	Health       *Health     `json:"health,omitempty"`
	Original     string      `json:"original,omitempty"`
	Value        interface{} `json:"value,omitempty"`
}

func ConvertBlock(
	id []string,
	args component.Arguments,
	exports component.Exports,
	references, referencedby []string,
	health *Health,
	original string,
) *Field {

	nf := &Field{}
	nf.Health = health
	nf.References = references
	nf.ReferencedBy = referencedby
	nf.Original = original
	nf.Type = "block"
	nf.ID = strings.Join(id, ".")
	nf.Name = strings.Join(id[0:2], ".")
	nf.Label = id[2]
	fields := make([]*Field, 0)
	cArgs := ConvertArguments(args)
	if args != nil {
		fields = append(fields, cArgs)
	}
	cExports := ConvertExports(exports)
	if exports != nil {
		fields = append(fields, cExports)
	}
	nf.Value = fields
	return nf

}

func ConvertArguments(args component.Arguments) *Field {
	return convertField(args, &rivertags.Field{
		Name:  []string{"arguments"},
		Index: nil,
		Flags: 0,
	})
}

func ConvertExports(exports component.Exports) *Field {
	return convertField(exports, &rivertags.Field{
		Name:  []string{"exports"},
		Index: nil,
		Flags: 0,
	})
}

// ConvertToField converts to a generic field for json
func ConvertToField(in interface{}, name string) *Field {
	return convertField(in, &rivertags.Field{
		Name:  []string{name},
		Index: nil,
		Flags: 0,
	})
}

func convertField(in interface{}, f *rivertags.Field) *Field {
	// Assume everything is an attr unless otherwise specified
	nf := &Field{
		Type: "attr",
	}
	if f != nil && len(f.Name) > 0 {
		nf.Key = f.Name[len(f.Name)-1]
	}

	nt := reflect.TypeOf(in)
	vIn := reflect.ValueOf(in)
	if in != nil {
		for nt.Kind() == reflect.Pointer && !vIn.IsZero() {
			vIn = vIn.Elem()
			nt = vIn.Type()
		}
		in = vIn.Interface()
	} else {
		nf.Value = &Field{
			Type: "null",
		}
		return nf
	}

	// Dont write zero value records
	if reflect.ValueOf(in).IsZero() {
		return nil
	}

	switch in.(type) {
	case rivertypes.Secret:
		nf.Value = &Field{
			Type:  "string",
			Value: "(secret)",
		}
		return nf
	case rivertypes.OptionalSecret:
		nf.Value = &Field{
			Type:  "string",
			Value: "(secret)",
		}
		maybeSecret := in.(rivertypes.OptionalSecret)
		if !maybeSecret.IsSecret {
			nf.Value.(*Field).Value = maybeSecret.Value
		}
		return nf

	}

	rt := value.RiverType(reflect.TypeOf(in))
	rv := value.NewValue(reflect.ValueOf(in), rt)
	switch rt {
	case value.TypeNull:
		nf.Value = &Field{
			Type: "null",
		}
		return nf
	case value.TypeNumber:
		numField := &Field{
			Type: "number",
		}
		switch value.MakeNumberKind(vIn.Kind()) {
		case value.NumberKindInt:
			numField.Value = rv.Int()
		case value.NumberKindUint:
			numField.Value = rv.Uint()
		case value.NumberKindFloat:
			numField.Value = rv.Float()
		}
		nf.Value = numField
		return nf
	case value.TypeString:
		nf.Value = &Field{
			Type:  "string",
			Value: rv.Text(),
		}
		return nf
	case value.TypeBool:
		nf.Value = &Field{
			Type:  "bool",
			Value: rv.Bool(),
		}
		return nf
	case value.TypeArray:
		nf.Type = "array"
		fields := make([]*Field, 0)
		for i := 0; i < vIn.Len(); i++ {
			arrEle := vIn.Index(i).Interface()
			found := convertField(arrEle, f)
			if found != nil {
				fields = append(fields, found)
			}
		}
		nf.Value = fields
		return nf
	case value.TypeObject:
		if vIn.Kind() == reflect.Struct {
			if f != nil && f.IsBlock() {
				nf.Type = "block"
				nf.ID = strings.Join(f.Name, ".")
				// remote_write "t1"
				if len(f.Name) == 2 {
					nf.Name = f.Name[0]
					if f.Name[1] != "" {
						nf.Label = f.Name[1]
					}
				}
			} else {
				nf.Type = "object"
			}

			fields := make([]*Field, 0)
			riverFields := rivertags.Get(reflect.TypeOf(in))
			for _, rf := range riverFields {
				fieldValue := vIn.FieldByIndex(rf.Index)
				found := convertField(fieldValue.Interface(), &rf)
				if found != nil {
					fields = append(fields, found)
				}
			}
			nf.Value = fields
			return nf
		} else if vIn.Kind() == reflect.Map {
			nf.Type = "map"
			fields := make([]*Field, 0)
			iter := vIn.MapRange()
			for iter.Next() {
				mf := &Field{}
				mf.Key = iter.Key().String()
				mf.Value = convertField(iter.Value().Interface(), nil)
				if mf.Value != nil {
					fields = append(fields, mf)
				}
			}
			nf.Value = fields
			return nf
		} else {
			if f.IsBlock() && f.IsOptional() {
				return nil
			}
			panic("wut?")
		}
	case value.TypeFunction:
		panic("func not handled")
	case value.TypeCapsule:
		nf.Type = "attr"
		nf.Value = &Field{
			Type:  "capsule",
			Value: rv.Describe(),
		}
		return nf
	}
	return nil
}
