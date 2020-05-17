package uri

import (
	"fmt"
	"testing"
)

func TestExpand(t *testing.T) {
	tests := []struct {
		raw  string
		args map[string]interface{}
		out  string
	}{
		{"http://localhost:8080/{id}", map[string]interface{}{"id": "foo"}, "http://localhost:8080/foo"},
		{"http://localhost:8080/{?date}", map[string]interface{}{"date": "2017-07-13"}, "http://localhost:8080/?date=2017-07-13"},
		{"http://localhost:8080/{?date,name}", map[string]interface{}{"date": "2017-07-13"}, "http://localhost:8080/?date=2017-07-13"},
		{"http://localhost:8080/{?date,name}", map[string]interface{}{"date": "2017-07-13", "name": "foo"}, "http://localhost:8080/?date=2017-07-13&name=foo"},
		{"http://localhost:8080/{?date,axle,distance}", map[string]interface{}{"date": "2017-07-13", "axle": 33, "distance": 34.7}, "http://localhost:8080/?date=2017-07-13&axle=33&distance=34.7"},
		{"{?sort,filter*,search,limit}", map[string]interface{}{"sort": "name,ASC"}, "?sort=name%2CASC"},
		{"{?sort,filter*,search,limit}", map[string]interface{}{"filter": "name,like,foo,bar"}, "?filter=name%2Clike%2Cfoo%2Cbar"},
		{"{?sort,filter*,search,limit}", map[string]interface{}{"sort": "name,ASC", "filter": "name,like,foo,bar"}, "?sort=name%2CASC&filter=name%2Clike%2Cfoo%2Cbar"},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			template, err := Parse(test.raw)
			if err != nil {
				t.Error(err)
			}
			out, err := template.Expand(test.args)
			if err != nil {
				t.Error(err)
			}
			if test.out != out {
				t.Errorf("want %s, got %s", test.out, out)
			}

		})
	}
}
