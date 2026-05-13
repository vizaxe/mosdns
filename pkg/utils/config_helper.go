/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package utils

import (
	"github.com/mitchellh/mapstructure"
	"golang.org/x/exp/constraints"
	"reflect"
	"strconv"
)

func SetDefaultNum[K constraints.Integer | constraints.Float](p *K, d K) {
	if *p == 0 {
		*p = d
	}
}

func SetDefaultUnsignNum[K constraints.Integer | constraints.Float](p *K, d K) {
	if *p <= 0 {
		*p = d
	}
}

func SetDefaultString(p *string, d string) {
	if len(*p) == 0 {
		*p = d
	}
}

func CheckNumRange[K constraints.Integer | constraints.Float](v, min, max K) bool {
	if v < min || v > max {
		return false
	}
	return true
}

// stringToStringSliceHook converts a single string to a slice of strings.
// This allows config fields of type []string to accept both a single string
// and a list of strings in YAML.
func stringToStringSliceHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() == reflect.String && t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.String {
			return []string{data.(string)}, nil
		}
		return data, nil
	}
}

// WeakDecode decodes args from config to output.
func WeakDecode(in any, output any) error {
	config := &mapstructure.DecoderConfig{
		ErrorUnused:      true,
		Result:           output,
		WeaklyTypedInput: true,
		TagName:          "yaml",
		DecodeHook:       stringToStringSliceHook(),
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(in)
}

func ParseNameOrNum[T constraints.Integer](s string, m map[string]T) (T, bool) {
	i, err := strconv.Atoi(s)
	if err != nil {
		v, ok := m[s]
		return v, ok
	}
	return T(i), true
}
