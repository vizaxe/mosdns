package geofile

import (
	"google.golang.org/protobuf/encoding/protowire"
)

type DomainType int32

const (
	Domain_Plain  DomainType = 0
	Domain_Regex  DomainType = 1
	Domain_Domain DomainType = 2
	Domain_Full   DomainType = 3
)

type CIDR struct {
	Ip     []byte
	Prefix uint32
}

type GeoIP struct {
	CountryCode  string
	Cidr         []*CIDR
	ReverseMatch bool
}

type Domain struct {
	Type  DomainType
	Value string
}

type GeoSite struct {
	CountryCode string
	Domain      []*Domain
}

func unmarshalGeoIP(b []byte) (*GeoIP, error) {
	g := &GeoIP{}
	for len(b) > 0 {
		num, wtype, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		b = b[n:]

		switch wtype {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			b = b[n:]
			if num == 3 {
				g.ReverseMatch = v != 0
			}
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			b = b[n:]
			switch num {
			case 1:
				g.CountryCode = string(v)
			case 2:
				c := &CIDR{}
				if err := unmarshalCIDR(c, v); err != nil {
					return nil, err
				}
				g.Cidr = append(g.Cidr, c)
			}
		default:
			n := protowire.ConsumeFieldValue(num, wtype, b)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return g, nil
}

func unmarshalCIDR(c *CIDR, b []byte) error {
	for len(b) > 0 {
		num, wtype, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]

		switch wtype {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
			if num == 2 {
				c.Prefix = uint32(v)
			}
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
			if num == 1 {
				c.Ip = v
			}
		default:
			n := protowire.ConsumeFieldValue(num, wtype, b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return nil
}

func unmarshalGeoSite(b []byte) (*GeoSite, error) {
	g := &GeoSite{}
	for len(b) > 0 {
		num, wtype, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		b = b[n:]

		switch wtype {
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			b = b[n:]
			switch num {
			case 1:
				g.CountryCode = string(v)
			case 2:
				d := &Domain{}
				if err := unmarshalDomain(d, v); err != nil {
					return nil, err
				}
				g.Domain = append(g.Domain, d)
			}
		default:
			n := protowire.ConsumeFieldValue(num, wtype, b)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return g, nil
}

func unmarshalDomain(d *Domain, b []byte) error {
	for len(b) > 0 {
		num, wtype, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]

		switch wtype {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
			if num == 1 {
				d.Type = DomainType(v)
			}
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
			if num == 2 {
				d.Value = string(v)
			}
		default:
			n := protowire.ConsumeFieldValue(num, wtype, b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return nil
}
