//     Copyright (C) 2020, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package handler

import (
	"fmt"
)

type Error struct {
	err string
}

func (e *Error) Error() string {
	return e.err
}

type ErrTemplate string

func NewErrFromTemplate(t ErrTemplate, args ...interface{}) error {
	return fmt.Errorf(string(t), args...)
}

const (
	ETInvalidArgs    = "invalid args: %w"
	ETPluginErr      = "plugin %s reported an err: %w"
	ETTypeNotDefined = "plugin type %s not defined"
	ETTagNotDefined  = "plugin tag %s not defined"
	ETInvalidQCtx    = "invalid qCtx: %w"
)