package expvarmon

import (
	"bytes"
	//"encoding/json"
	"expvar"
	"fmt"
	"github.com/gorilla/websocket"
	"html/template"
	"log"
	"net/http"
	"runtime"
	"sort"
	"sync"
	"time"
)

type update struct {
	Ts     int64
	Values map[string]string
}

type consumer struct {
	id uint
	c  chan update
}

type server struct {
	consumers      []consumer
	consumersMutex sync.RWMutex
}

const (
	maxCount int = 60 * 60 * 11 //86400
)

type dataStorageItems []*struct {
	ts    int64
	value string
}

func (items dataStorageItems) String(since int64) string {
	var buffer bytes.Buffer
	first := true
	buffer.WriteString("[")
	for i := 0; i < len(items); i++ {
		if items[i].ts > since {
			if first {
				first = false
			} else {
				buffer.WriteString(",")
			}
			buffer.WriteString(fmt.Sprintf("[%v, %v]", items[i].ts, items[i].value))
		}
	}
	buffer.WriteString("]")
	return buffer.String()
}

var (
	dataStorage = make(map[string]dataStorageItems)
	infoStorage = make(map[string]struct {
		name           string
		valueUnit      string
		valueUnitShort string
	})

	mutex          sync.RWMutex
	lastConsumerId uint
	s              server
	upgrader       = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

func RegisterVariableInfo(name, fullName, valueUnit, valueUnitShort string) {
	mutex.Lock()
	defer mutex.Unlock()
	if _, ok := infoStorage[name]; !ok {
		infoStorage[name] = struct {
			name           string
			valueUnit      string
			valueUnitShort string
		}{
			name:           fullName,
			valueUnit:      valueUnit,
			valueUnitShort: valueUnitShort,
		}
	}
}
func (s *server) gatherData() {
	memAlloc := expvar.NewInt("MemStats_Alloc")
	memTotalAlloc := expvar.NewInt("MemStats_TotalAlloc")
	memSys := expvar.NewInt("MemStats_Sys")
	memLookups := expvar.NewInt("MemStats_Lookups")
	memMallocs := expvar.NewInt("MemStats_Mallocs")
	memFrees := expvar.NewInt("MemStats_Frees")

	memHeapAlloc := expvar.NewInt("MemStats_HeapAlloc")
	memHeapSys := expvar.NewInt("MemStats_HeapSys")
	memHeapIdle := expvar.NewInt("MemStats_HeapIdle")
	memHeapInuse := expvar.NewInt("MemStats_HeapInuse")
	memHeapReleased := expvar.NewInt("MemStats_HeapReleased")
	memHeapObjects := expvar.NewInt("MemStats_HeapObjects")

	memStackInuse := expvar.NewInt("MemStats_StackInuse")
	memStackSys := expvar.NewInt("MemStats_StackSys")
	memMSpanInuse := expvar.NewInt("MemStats_MSpanInuse")
	memMSpanSys := expvar.NewInt("MemStats_MSpanSys")
	memMCacheInuse := expvar.NewInt("MemStats_MCacheInuse")
	memMCacheSys := expvar.NewInt("MemStats_MCacheSys")
	memBuckHashSys := expvar.NewInt("MemStats_BuckHashSys")
	memGCSys := expvar.NewInt("MemStats_GCSys")
	memOtherSys := expvar.NewInt("MemStats_OtherSys")

	memPauseNs := expvar.NewInt("MemStats_PauseNs")
	numGoroutine := expvar.NewInt("Stats_NumGoroutine")

	RegisterVariableInfo("MemStats_Alloc", "General statistics - bytes allocated and not yet freed", "Bytes", "b")
	RegisterVariableInfo("MemStats_TotalAlloc", "General statistics - bytes allocated (even if freed)", "Bytes", "b")
	RegisterVariableInfo("MemStats_Sys", "General statistics - bytes obtained from system (sum of XxxSys below)", "Bytes", "b")
	RegisterVariableInfo("MemStats_Lookups", "General statistics - number of pointer lookups", "Pieces", "p")
	RegisterVariableInfo("MemStats_Mallocs", "General statistics - number of mallocs", "Pieces", "p")
	RegisterVariableInfo("MemStats_Frees", "General statistics - number of frees", "Pieces", "p")

	RegisterVariableInfo("MemStats_HeapAlloc", "Main allocation heap statistics - bytes allocated and not yet freed (same as Alloc above)", "Bytes", "b")
	RegisterVariableInfo("MemStats_HeapSys", "Main allocation heap statistics - bytes obtained from system", "Bytes", "b")
	RegisterVariableInfo("MemStats_HeapIdle", "Main allocation heap statistics - bytes in idle spans", "Bytes", "b")
	RegisterVariableInfo("MemStats_HeapInuse", "Main allocation heap statistics - bytes in non-idle span", "Bytes", "b")
	RegisterVariableInfo("MemStats_HeapReleased", "Main allocation heap statistics - bytes released to the OS", "Bytes", "b")
	RegisterVariableInfo("MemStats_HeapObjects", "Main allocation heap statistics - total number of allocated objects", "Pieces", "p")

	RegisterVariableInfo("MemStats_StackInuse", "LowLevel - bytes used by stack allocator - in use now", "Bytes", "b")
	RegisterVariableInfo("MemStats_StackSys", "LowLevel - bytes used by stack allocator  - obtained from sys", "Bytes", "b")
	RegisterVariableInfo("MemStats_MSpanInuse", "LowLevel - mspan structures - in use now", "Bytes", "b")
	RegisterVariableInfo("MemStats_MSpanSys", "LowLevel - mspan structures - obtained from sys", "Bytes", "b")
	RegisterVariableInfo("MemStats_MCacheInuse", "LowLevel - mcache structures - in use now", "Bytes", "b")
	RegisterVariableInfo("MemStats_MCacheSys", "LowLevel - mcache structures - obtained from sys", "Bytes", "b")
	RegisterVariableInfo("MemStats_BuckHashSys", "LowLevel - profiling bucket hash table - obtained from sys", "Bytes", "b")
	RegisterVariableInfo("MemStats_GCSys", "LowLevel - GC metadata - obtained from sys", "Bytes", "b")
	RegisterVariableInfo("MemStats_OtherSys", "LowLevel - other system allocations - obtained from sys", "Bytes", "b")

	RegisterVariableInfo("MemStats_PauseNs", "GC pause duration", "Nanoseconds", "ns")
	RegisterVariableInfo("Stats_NumGoroutine", "Number of goroutines", "Goroutines", "g")

	timer := time.Tick(time.Second)
	for {
		select {
		case now := <-timer:
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)

			memAlloc.Set(int64(ms.Alloc))
			memTotalAlloc.Set(int64(ms.TotalAlloc))
			memSys.Set(int64(ms.Sys))
			memLookups.Set(int64(ms.Lookups))
			memMallocs.Set(int64(ms.Mallocs))
			memFrees.Set(int64(ms.Frees))

			memHeapAlloc.Set(int64(ms.HeapAlloc))
			memHeapSys.Set(int64(ms.HeapSys))
			memHeapIdle.Set(int64(ms.HeapIdle))
			memHeapInuse.Set(int64(ms.HeapInuse))
			memHeapReleased.Set(int64(ms.HeapReleased))
			memHeapObjects.Set(int64(ms.HeapObjects))

			memStackInuse.Set(int64(ms.StackInuse))
			memStackSys.Set(int64(ms.StackSys))
			memMSpanInuse.Set(int64(ms.MSpanInuse))
			memMSpanSys.Set(int64(ms.MSpanSys))
			memMCacheInuse.Set(int64(ms.MCacheInuse))
			memMCacheSys.Set(int64(ms.MCacheSys))
			memBuckHashSys.Set(int64(ms.BuckHashSys))
			memGCSys.Set(int64(ms.GCSys))
			memOtherSys.Set(int64(ms.OtherSys))

			memPauseNs.Set(int64(ms.PauseNs[(ms.NumGC+255)%256]))
			numGoroutine.Set(int64(runtime.NumGoroutine()))

			func() {
				u := update{
					Ts:     now.Unix() * 1000,
					Values: make(map[string]string),
				}

				func() {
					mutex.Lock()
					defer mutex.Unlock()

					for k, _ := range infoStorage {
						d, ok := dataStorage[k]
						if !ok {
							d = make(dataStorageItems, 0, maxCount)
						}
						v := expvar.Get(k)
						if v != nil {
							s := v.String()
							d = append(d, &struct {
								ts    int64
								value string
							}{
								ts:    now.Unix() * 1000,
								value: s,
							})
							if len(d) > maxCount {
								for i := 0; i < len(d)-maxCount; i++ {
									d[i] = nil
								}
								d = d[len(d)-maxCount:]
							}
							dataStorage[k] = d
							u.Values[k] = s
						} else {
							fmt.Println(k)
						}
					}
				}()
				s.sendToConsumers(u)
			}()

		}
	}
}

func (s *server) sendToConsumers(u update) {
	s.consumersMutex.RLock()
	defer s.consumersMutex.RUnlock()

	for _, c := range s.consumers {
		c.c <- u
	}
}

func (s *server) removeConsumer(id uint) {
	s.consumersMutex.Lock()
	defer s.consumersMutex.Unlock()

	var consumerId uint
	var consumerFound bool

	for i, c := range s.consumers {
		if c.id == id {
			consumerFound = true
			consumerId = uint(i)
			break
		}
	}

	if consumerFound {
		s.consumers = append(s.consumers[:consumerId], s.consumers[consumerId+1:]...)
	}
}

func (s *server) addConsumer() consumer {
	s.consumersMutex.Lock()
	defer s.consumersMutex.Unlock()

	lastConsumerId += 1

	c := consumer{
		id: lastConsumerId,
		c:  make(chan update),
	}

	s.consumers = append(s.consumers, c)

	return c
}

func (s *server) dataFeedHandler(w http.ResponseWriter, r *http.Request) {
	var (
		lastPing time.Time
		lastPong time.Time
	)

	varName := r.FormValue("var")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error: ", err)
		return
	}

	conn.SetPongHandler(func(s string) error {
		lastPong = time.Now()
		return nil
	})

	// read and discard all messages
	go func(c *websocket.Conn) {
		for {
			if _, _, err := c.NextReader(); err != nil {
				c.Close()
				break
			}
		}
	}(conn)

	c := s.addConsumer()

	defer func() {
		s.removeConsumer(c.id)
		conn.Close()
	}()

	var i uint

	for u := range c.c {
		var buffer bytes.Buffer
		buffer.WriteString("{\n")
		buffer.WriteString(fmt.Sprintf("\"Ts\": %v\n,\"Value\": %s", u.Ts, u.Values[varName]))
		buffer.WriteString("\n}\n")
		conn.WriteMessage(websocket.TextMessage, buffer.Bytes())

		i += 1

		if i%10 == 0 {
			if diff := lastPing.Sub(lastPong); diff > time.Second*60 {
				return
			}
			now := time.Now()
			if err := conn.WriteControl(websocket.PingMessage, nil, now.Add(time.Second)); err != nil {
				return
			}
			lastPing = now
		}
	}
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	defer mutex.RUnlock()

	if e := r.ParseForm(); e != nil {
		log.Fatalln("error pardsing form")
	}

	callback := r.FormValue("callback")
	fmt.Fprintf(w, "%v(", callback)

	w.Header().Set("Content-Type", "application/json")

	fmt.Fprintf(w, "{\n")
	mutex.RLock()
	defer mutex.RUnlock()

	fmt.Fprintf(w, "\"ts\": %v", time.Now().Unix()*1000)

	varName := r.FormValue("var")

	for k, _ := range dataStorage {
		if (varName == "") || (varName == k) {
			fmt.Fprintf(w, ",\n%q: %s", k, dataStorage[k].String(0))
		}
	}

	fmt.Fprintf(w, "\n}\n")

	fmt.Fprint(w, ")")
}

type Vars []struct {
	Var      string
	Name     string
	Selected int
}

func (v Vars) Len() int { return len(v) }

func (v Vars) Swap(i, j int) { v[i], v[j] = v[j], v[i] }

func (v Vars) Less(i, j int) bool { return v[i].Name < v[j].Name }

func handleTemplate(templateName, templateBody string) func(http.ResponseWriter, *http.Request) {
	templ, err := template.New(templateName).Parse(templateBody)
	if err != nil {
		panic(err)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		mutex.RLock()
		defer mutex.RUnlock()
		type _T struct {
			Vars []struct {
				Var      string
				Name     string
				Selected int
			}
			Var            string
			Name           string
			ValueUnit      string
			ValueUnitShort string
		}
		t := _T{Var: r.FormValue("var"), Vars: make(Vars, 0)}

		i, ok := infoStorage[r.FormValue("var")]
		if !ok {
			t.Name = r.FormValue("var")
			t.ValueUnit = ""
			t.ValueUnitShort = ""
		} else {
			t.Name = i.name
			t.ValueUnit = i.valueUnit
			t.ValueUnitShort = i.valueUnitShort
		}

		for k, _ := range infoStorage {
			if k == t.Var {
				t.Vars = append(t.Vars, struct {
					Var      string
					Name     string
					Selected int
				}{
					Var:      k,
					Name:     infoStorage[k].name,
					Selected: 1,
				})
			} else {
				t.Vars = append(t.Vars, struct {
					Var      string
					Name     string
					Selected int
				}{
					Var:      k,
					Name:     infoStorage[k].name,
					Selected: 0,
				})
			}
		}
		sort.Sort(Vars(t.Vars))

		//		expvar.Do(func(kv expvar.KeyValue) {
		//			if vn, ok := infoStorage[kv.Key]; ok {
		//				if kv.Key == t.Var {
		//					t.Vars = append(t.Vars, struct {
		//						Var      string
		//						Name     string
		//						Selected int
		//					}{
		//						Var:      kv.Key,
		//						Name:     vn.name,
		//						Selected: 1,
		//					})
		//				} else {
		//					t.Vars = append(t.Vars, struct {
		//						Var      string
		//						Name     string
		//						Selected int
		//					}{
		//						Var:      kv.Key,
		//						Name:     vn.name,
		//						Selected: 0,
		//					})
		//				}
		//			}
		//		})
		if r.FormValue("var") == "" {
			t.Vars = append(t.Vars, struct {
				Var      string
				Name     string
				Selected int
			}{
				Var:      "",
				Name:     "",
				Selected: 1,
			})
		}

		err = templ.ExecuteTemplate(w, templateName, t)
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(200)
			fmt.Fprint(w, "Error:", err)
			return
		}
	}
}

func init() {
	http.HandleFunc("/debug/metrics/data-feed", s.dataFeedHandler)
	http.HandleFunc("/debug/metrics/data", dataHandler)
	http.HandleFunc("/debug/metrics/main.html", handleTemplate("mainPage", mainPage))
	http.HandleFunc("/debug/metrics/main.js", handleTemplate("mainJs", mainJs))
	go s.gatherData()
}

const (
	mainPage = `
<!DOCTYPE html>
<html>
	<head>
		<title></title>
		<meta charset="utf-8" />
		<script src="http://code.jquery.com/jquery-1.11.0.min.js"></script>
		<script src="http://code.highcharts.com/stock/highstock.js"></script>
		<script src="http://code.highcharts.com/stock/modules/exporting.js"></script>
		<script>
		function doNavigate(varName){
			window.location.assign("/debug/metrics/main.html?var="+varName);
		}
		</script>
	</head>
	<body>
Select variable: <select onChange="javascript:doNavigate(this.value);">>
{{range $key, $data := .Vars}}
<option value="{{$data.Var}}" {{if eq $data.Selected 1}}selected{{end}}>{{$data.Name}}</option>
{{end}}
</select>
		<div id="container" style="min-width: 310px; height: 400px; margin: 0 auto"></div>
		<script src="main.js?var={{.Var}}"></script>
	</body>
</html>`

	mainJs = `
var chartD;


$(function() {
	var x = new Date();

	Highcharts.setOptions({
		global: {
			timezoneOffset: x.getTimezoneOffset()
		}
	})

	$.getJSON('/debug/metrics/data?callback=?&var={{.Var}}', function(data) {
		chartD = new Highcharts.StockChart({
			chart: {
				renderTo: 'container',
				zoomType: 'x'
			},
			title: {
				text: '{{.Name}}'
			},
			yAxis: {
				title: {
					text: '{{.ValueUnit}}'
				}
			},
			scrollbar: {
				enabled: false
			},
			rangeSelector: {
				buttons: [{
					type: 'second',
					count: 5,
					text: '5s'
				}, {
					type: 'second',
					count: 30,
					text: '30s'
				}, {
					type: 'minute',
					count: 1,
					text: '1m'
				}, {
					type: 'all',
					text: 'All'
				}],
				selected: 3
			},
			series: [{
				name: "{{.Name}}",
				data: data.{{.Var}},
				type: 'area',
				tooltip: {
					valueSuffix: '{{.ValueUnitShort}}'
				}
			}]
		})
	});


	function wsurl() {
		var l = window.location;
		return ((l.protocol === "https:") ? "wss://" : "ws://") + l.hostname + (((l.port != 80) && (l.port != 443)) ? ":" + l.port : "") + "/debug/metrics/data-feed?var={{.Var}}";
	}

	ws = new WebSocket(wsurl());
	ws.onopen = function () {
		ws.onmessage = function (evt) {
			var data = JSON.parse(evt.data);
			chartD.series[0].addPoint([data.Ts, data.Value], true);
		}
	};
})
`
)
