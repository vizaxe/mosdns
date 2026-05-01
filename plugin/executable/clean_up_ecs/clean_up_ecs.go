/*
 * Copyright (C) 2024, Vizaxe
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

package clean_up_ecs

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

func init() {
	sequence.MustRegExecQuickSetup("clean_up_ecs", func(bq sequence.BQ, args string) (any, error) {
		return &cleanUpECS{}, nil
	})
}

var _ sequence.RecursiveExecutable = (*cleanUpECS)(nil)

type cleanUpECS struct {
}

func (e *cleanUpECS) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	queryOpt := qCtx.QOpt()
	for i := 0; i < len(queryOpt.Option); i++ {
		if queryOpt.Option[i].Option() == dns.EDNS0SUBNET {
			queryOpt.Option = append(queryOpt.Option[:i], queryOpt.Option[i+1:]...)
			i--
		}
	}
	err := next.ExecNext(ctx, qCtx)
	if err != nil {
		return err
	}
	return nil
}
