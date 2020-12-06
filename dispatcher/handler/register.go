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
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sync"
)

type NewPluginFunc func(tag string, args map[string]interface{}) (p Plugin, err error)

var (
	// pluginTypeRegister stores init funcs for all plugin types
	pluginTypeRegister = make(map[string]NewPluginFunc)

	// templateArgsRegister stores plugin template args. It's optional.
	templateArgsRegister = make(map[string]interface{})

	pluginTagRegister = newPluginRegister()

	entryTagRegister = &entryRegister{}
)

func SetTemArgs(typ string, args interface{}) {
	templateArgsRegister[typ] = args
}

func GetTemArgs(typ string) (args interface{}, ok bool) {
	args, ok = templateArgsRegister[typ]
	return
}

type entryRegister struct {
	sync.RWMutex
	e []string
}

func (r *entryRegister) reg(entry ...string) {
	r.Lock()
	r.e = append(r.e[0:len(r.e):len(r.e)], entry...) // will always allocate a new memory
	r.Unlock()
}

func (r *entryRegister) get() (e []string) {
	r.RLock()
	e = r.e
	r.RUnlock()
	return
}

func (r *entryRegister) purge() {
	r.Lock()
	r.e = nil
	r.Unlock()
	return
}

func (r *entryRegister) del(entryRegexp string) (deleted []string, err error) {
	expr, err := regexp.Compile(entryRegexp)
	if err != nil {
		return nil, err
	}

	remain := make([]string, 0, len(r.e))
	r.RLock()
	for _, entry := range r.e {
		if expr.MatchString(entry) { // del it
			deleted = append(deleted, entry)
			continue
		}
		remain = append(remain, entry)
	}
	r.RUnlock()

	r.Lock()
	r.e = remain
	r.Unlock()

	return deleted, nil
}

type pluginRegister struct {
	sync.RWMutex
	register map[string]Plugin
}

func newPluginRegister() *pluginRegister {
	return &pluginRegister{
		register: make(map[string]Plugin),
	}
}

func (r *pluginRegister) regPlugin(p Plugin, panicIfErr bool) error {
	switch p.(type) {
	case FunctionalPlugin, MatcherPlugin, RouterPlugin:
		r.Lock()
		r.register[p.Tag()] = p
		r.Unlock()
	default:
		err := fmt.Errorf("unexpected plugin interface type %s", reflect.TypeOf(p).String())
		if panicIfErr {
			panic(err.Error())
		}
		return err
	}
	return nil
}

func (r *pluginRegister) getFunctionalPlugin(tag string) (p FunctionalPlugin, ok bool) {
	r.RLock()
	defer r.RUnlock()
	if gp, ok := r.register[tag]; ok {
		if p, ok := gp.(FunctionalPlugin); ok {
			return p, true
		}
	}

	return
}
func (r *pluginRegister) getMatcherPlugin(tag string) (p MatcherPlugin, ok bool) {
	r.RLock()
	defer r.RUnlock()
	if gp, ok := r.register[tag]; ok {
		if p, ok := gp.(MatcherPlugin); ok {
			return p, true
		}
	}
	return
}

func (r *pluginRegister) getRouterPlugin(tag string) (p RouterPlugin, ok bool) {
	r.RLock()
	defer r.RUnlock()
	if gp, ok := r.register[tag]; ok {
		if p, ok := gp.(RouterPlugin); ok {
			return p, true
		}
	}
	return
}

func (r *pluginRegister) getPlugin(tag string) (p Plugin, ok bool) {
	r.RLock()
	defer r.RUnlock()
	p, ok = r.register[tag]
	return
}

func (r *pluginRegister) purge() {
	r.Lock()
	r.register = make(map[string]Plugin)
	r.Unlock()
}

// RegInitFunc registers this plugin type.
// This should only be called in init() of the plugin package.
// Duplicate plugin types are not allowed.
func RegInitFunc(pluginType string, initFunc NewPluginFunc) {
	_, ok := pluginTypeRegister[pluginType]
	if ok {
		panic(fmt.Sprintf("duplicate plugin type [%s]", pluginType))
	}
	pluginTypeRegister[pluginType] = initFunc
}

// GetPluginTypes returns all registered plugin types.
func GetPluginTypes() []string {
	b := make([]string, 0, len(pluginTypeRegister))
	for typ := range pluginTypeRegister {
		b = append(b, typ)
	}
	return b
}

// InitAndRegPlugin inits and registers this plugin globally.
// Duplicate plugin tags are not allowed.
func InitAndRegPlugin(c *Config) (err error) {
	p, err := NewPlugin(c)
	if err != nil {
		return fmt.Errorf("failed to init plugin [%s], %w", c.Tag, err)
	}

	return RegPlugin(p)
}

func NewPlugin(c *Config) (p Plugin, err error) {
	newPluginFunc, ok := pluginTypeRegister[c.Type]
	if !ok {
		return nil, NewErrFromTemplate(ETTypeNotDefined, c.Type)
	}

	return newPluginFunc(c.Tag, c.Args)
}

// RegPlugin registers this Plugin globally.
// Duplicate Plugin tag will be overwritten.
// Plugin must be a FunctionalPlugin, MatcherPlugin or RouterPlugin.
func RegPlugin(p Plugin) error {
	return pluginTagRegister.regPlugin(p, false)
}

func MustRegPlugin(p Plugin) {
	pluginTagRegister.regPlugin(p, true)
}

func GetPlugin(tag string) (p Plugin, ok bool) {
	return pluginTagRegister.getPlugin(tag)
}

func GetFunctionalPlugin(tag string) (p FunctionalPlugin, ok bool) {
	return pluginTagRegister.getFunctionalPlugin(tag)
}

func GetMatcherPlugin(tag string) (p MatcherPlugin, ok bool) {
	return pluginTagRegister.getMatcherPlugin(tag)
}

func GetRouterPlugin(tag string) (p RouterPlugin, ok bool) {
	return pluginTagRegister.getRouterPlugin(tag)
}

// PurgePluginRegister should only be used in test.
func PurgePluginRegister() {
	pluginTagRegister.purge()
}

func RegEntry(entry ...string) {
	entryTagRegister.reg(entry...)
}

func DelEntry(entryRegexp string) (deleted []string, err error) {
	return entryTagRegister.del(entryRegexp)
}

func GetEntry() []string {
	return entryTagRegister.get()
}

func PurgeEntry() {
	entryTagRegister.purge()
}

func Dispatch(ctx context.Context, qCtx *Context) error {
	return entryTagRegister.dispatch(ctx, qCtx)
}