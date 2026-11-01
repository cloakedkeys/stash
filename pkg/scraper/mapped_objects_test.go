package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antchfx/htmlquery"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestMappedXPathSingleObjectSkipsEmptyMatches(t *testing.T) {
	for _, tt := range []struct {
		name string
		html string
		attr mappedScraperAttrConfig
		want string
	}{
		{"concat", "<div>a<span></span> \n <span></span>b</div>", mappedScraperAttrConfig{Selector: "//div/text()", Concat: ", "}, "a, b"},
		{"split", "<div> \n <span></span>a|b</div>", mappedScraperAttrConfig{Selector: "//div/text()", Split: "|"}, "a"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := htmlquery.Parse(strings.NewReader(tt.html))
			require.NoError(t, err)
			s := mappedScraper{Scene: &mappedSceneScraperConfig{mappedConfig: mappedConfig{"Title": tt.attr}}}
			scene, err := s.scrapeScene(context.Background(), &xpathQuery{doc: doc})
			require.NoError(t, err)
			require.NotNil(t, scene)
			require.NotNil(t, scene.Title)
			require.Equal(t, tt.want, *scene.Title)
		})
	}
}

func TestMappedXPathSingleObjectSubScrapeSkipsEmptyMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<div> \n <span></span>title</div>"))
	}))
	defer server.Close()
	doc, err := htmlquery.Parse(strings.NewReader(`<a href="` + server.URL + `">link</a>`))
	require.NoError(t, err)
	action := postProcessSubScraper{Selector: "//div/text()"}
	s := mappedScraper{Scene: &mappedSceneScraperConfig{mappedConfig: mappedConfig{
		"Title": {Selector: "//a/@href", postProcessActions: []postProcessAction{&action}},
	}}}
	q := &xpathQuery{doc: doc, scraper: &xpathScraper{client: server.Client(), globalConfig: mockGlobalConfig{}}}
	scene, err := s.scrapeScene(context.Background(), q)
	require.NoError(t, err)
	require.NotNil(t, scene)
	require.NotNil(t, scene.Title)
	require.Equal(t, "title", *scene.Title)
}

func TestMappedXPathObjectConcatSkipsEmptyMatches(t *testing.T) {
	for _, split := range []string{"", "|"} {
		t.Run("split="+split, func(t *testing.T) {
			doc, err := htmlquery.Parse(strings.NewReader("<div>Jane<span></span> \n <span></span>John</div><p>Female</p><p>Male</p>"))
			require.NoError(t, err)
			s := mappedScraper{Image: &mappedImageScraperConfig{Performers: mappedConfig{
				"Name":   {Selector: "//div/text()", Concat: "|", Split: split},
				"Gender": {Selector: "//p/text()", Concat: "|", Split: split},
			}}}
			image, err := s.scrapeImage(context.Background(), &xpathQuery{doc: doc})
			require.NoError(t, err)
			require.NotNil(t, image)
			want := []string{"Jane|John"}
			if split != "" {
				want = []string{"Jane", "John"}
			}
			require.Len(t, image.Performers, len(want))
			for i, name := range want {
				require.Equal(t, name, *image.Performers[i].Name)
				gender := "Female|Male"
				if split != "" {
					gender = []string{"Female", "Male"}[i]
				}
				require.NotNil(t, image.Performers[i].Gender)
				require.Equal(t, gender, *image.Performers[i].Gender)
			}
		})
	}
}

func TestMappedXPathObjectTextNamesStayAligned(t *testing.T) {
	doc, err := htmlquery.Parse(strings.NewReader(`<a href="/jane">
  <img src="x"> Jane
</a>
<a href="/john">
  <img src="y"> John
</a>`))
	require.NoError(t, err)
	s := mappedScraper{Image: &mappedImageScraperConfig{Performers: mappedConfig{
		"Name": {Selector: "//a/text()"},
		"URLs": {Selector: "//a/@href"},
	}}}
	image, err := s.scrapeImage(context.Background(), &xpathQuery{doc: doc})
	require.NoError(t, err)
	require.NotNil(t, image)
	require.Len(t, image.Performers, 2)
	urls := []string{"/jane", "/john"}
	for i, name := range []string{"Jane", "John"} {
		require.NotNil(t, image.Performers[i].Name)
		require.Equal(t, name, *image.Performers[i].Name)
		require.Equal(t, []string{urls[i]}, image.Performers[i].URLs)
	}
}

func TestMappedObjectAttributesStayAligned(t *testing.T) {
	const jsonDoc = `{"objects":[
		{"name":"Jane","value":"Female","url":"/jane"},
		{"name":"Jane 2","value":"Female","url":"/jane2"},
		{"name":"Jane","value":"Other","url":"/duplicate"},
		{"name":"","value":"Other","url":"/unnamed"},
		{"name":"Unknown","value":"","url":""},
		{"name":"John","value":"Male","url":"/john"}]}`
	const htmlDoc = `<div>
		<p data-name="Jane" data-value="Female" data-url="/jane"></p>
		<p data-name="Jane 2" data-value="Female" data-url="/jane2"></p>
		<p data-name="Jane" data-value="Other" data-url="/duplicate"></p>
		<p data-name="" data-value="Other" data-url="/unnamed"></p>
		<p data-name="Unknown" data-value="" data-url=""></p>
		<p data-name="John" data-value="Male" data-url="/john"></p>
	</div>`
	for _, engine := range []string{"json", "xpath"} {
		t.Run(engine, func(t *testing.T) {
			var q mappedQuery
			selector := func(field string) string { return "objects.#." + field }
			if engine == "json" {
				q = &jsonQuery{doc: jsonDoc}
			} else {
				doc, err := htmlquery.Parse(strings.NewReader(htmlDoc))
				require.NoError(t, err)
				q = &xpathQuery{doc: doc}
				selector = func(field string) string { return "//p/@data-" + field }
			}
			config := mappedConfig{
				"Name":     {Selector: selector("name")},
				"Gender":   {Selector: selector("value")},
				"Director": {Selector: selector("value")},
				"URLs":     {Selector: selector("url")},
				"Images":   {Selector: selector("url")},
			}
			s := mappedScraper{
				Scene: &mappedSceneScraperConfig{
					Performers: mappedPerformerScraperConfig{mappedConfig: config},
					Movies:     config,
					Groups:     config,
				},
				Image:   &mappedImageScraperConfig{Performers: config},
				Gallery: &mappedGalleryScraperConfig{Performers: config},
			}
			ctx := context.Background()
			scene, err := s.scrapeScene(ctx, q)
			require.NoError(t, err)
			require.NotNil(t, scene)
			image, err := s.scrapeImage(ctx, q)
			require.NoError(t, err)
			require.NotNil(t, image)
			gallery, err := s.scrapeGallery(ctx, q)
			require.NoError(t, err)
			require.NotNil(t, gallery)

			names := []string{"Jane", "Jane 2", "Unknown", "John"}
			values := []string{"Female", "Female", "", "Male"}
			urls := []string{"/jane", "/jane2", "", "/john"}
			for _, performers := range [][]*models.ScrapedPerformer{scene.Performers, image.Performers, gallery.Performers} {
				require.Len(t, performers, len(names))
				for i, p := range performers {
					require.Equal(t, names[i], *p.Name)
					if values[i] != "" {
						require.NotNil(t, p.Gender)
						require.Equal(t, values[i], *p.Gender)
					} else {
						require.Nil(t, p.Gender)
					}
					if urls[i] != "" {
						require.Equal(t, []string{urls[i]}, p.URLs)
						require.Equal(t, []string{urls[i]}, p.Images)
					} else {
						require.Empty(t, p.URLs)
						require.Empty(t, p.Images)
					}
				}
			}
			require.Len(t, scene.Movies, len(names))
			require.Len(t, scene.Groups, len(names))
			for i := range names {
				require.Equal(t, names[i], *scene.Movies[i].Name)
				require.Equal(t, names[i], *scene.Groups[i].Name)
				if values[i] != "" {
					require.Equal(t, values[i], *scene.Movies[i].Director)
					require.Equal(t, values[i], *scene.Groups[i].Director)
				} else {
					require.Nil(t, scene.Movies[i].Director)
					require.Nil(t, scene.Groups[i].Director)
				}
			}
		})
	}
}

func TestMappedObjectSplitAttributesStayAligned(t *testing.T) {
	for _, concat := range []bool{false, true} {
		name := "split"
		doc := `{"names":"Jane|Jane 2|Unknown|John","genders":"Female|Female||Male"}`
		separator := ""
		if concat {
			name = "concat and split"
			doc = `{"names":["Jane","Jane 2","Unknown","John"],"genders":["Female","Female","","Male"]}`
			separator = "|"
		}
		t.Run(name, func(t *testing.T) {
			s := mappedScraper{Image: &mappedImageScraperConfig{Performers: mappedConfig{
				"Name":   {Selector: "names", Split: "|", Concat: separator},
				"Gender": {Selector: "genders", Split: "|", Concat: separator},
			}}}
			image, err := s.scrapeImage(context.Background(), &jsonQuery{doc: doc})
			require.NoError(t, err)
			require.NotNil(t, image)
			require.Len(t, image.Performers, 4)
			for i, gender := range []string{"Female", "Female", "", "Male"} {
				if gender == "" {
					require.Nil(t, image.Performers[i].Gender)
				} else {
					require.NotNil(t, image.Performers[i].Gender)
					require.Equal(t, gender, *image.Performers[i].Gender)
				}
			}
		})
	}
}
