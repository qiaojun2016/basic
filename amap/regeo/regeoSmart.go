package regeo

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/qiaojun2016/basic/request"
	"log"
	"sort"
	"strconv"
	"strings"
)

//逆地理编码API服务地址，坐标转名称

type (

	//解析原始数据
	reGeoSmartRes struct {
		Status    string         `json:"status"` //"1"成功
		Info      string         `json:"info"`
		ReGeocode reGeoSmartCode `json:"regeocode"`
	}

	reGeoSmartCode struct {
		FormattedAddress string                `json:"formatted_address"` //结构化地址信息
		AddressComponent addressComponentSmart `json:"addressComponent"`  //地址元素列表
		Roads            []roadSmart           `json:"roads"`             //道路信息列表
		Pois             []poiSmart            `json:"pois"`              //poi信息列表，兴趣点
	}

	addressComponentSmart struct {
		Province  string      `json:"province"` //省
		ICity     interface{} `json:"city"`     //市
		City      string      //市
		IDistrict interface{} `json:"district"` //区
		District  string      //区
		ITownship interface{} `json:"township"` //乡镇/街道
		Township  string      //乡镇/街道
	}

	roadSmart struct {
		Name     string `json:"name"` //道路名称
		Location string `json:"location"`
	}

	poiSmart struct {
		Name      string      `json:"name"`
		IAddress  interface{} `json:"address"`
		Address   string
		Location  string `json:"location"`
		ODistance string `json:"distance"`
		Distance  float64
	}

	PoiSmartItem struct {
		Place    string `json:"place"`
		Address  string `json:"address"`
		Location string `json:"location"`
	}

	// ReGeoSmartRes 返回数据
	ReGeoSmartRes struct {
		FormattedAddress string `json:"formatted_address"`
		Province         string `json:"province"` //省
		City             string `json:"city"`     //市
		District         string `json:"district"` //区
		Township         string `json:"township"` //乡镇/街道
		Address          string `json:"address"`
		Place            string `json:"place"`
		Location         string `json:"location"`
	}
	ReGeoSmartListRes struct {
		FormattedAddress string         `json:"formatted_address"`
		Province         string         `json:"province"` //省
		City             string         `json:"city"`     //市
		District         string         `json:"district"` //区
		Township         string         `json:"township"` //乡镇/街道
		Pois             []PoiSmartItem `json:"pois"`
	}
)

func ReGeoSmartList(key, location string) (res ReGeoSmartListRes, err error) {
	//解析
	resp, err := _ReGeoSmart(key, location)
	if err != nil {
		log.Println(err)
		return
	}
	res.FormattedAddress = resp.ReGeocode.FormattedAddress
	res.Province = resp.ReGeocode.AddressComponent.Province
	res.City = resp.ReGeocode.AddressComponent.City
	res.District = resp.ReGeocode.AddressComponent.District
	res.Township = resp.ReGeocode.AddressComponent.Township

	//POI
	if len(resp.ReGeocode.Pois) > 0 {
		for _, poi := range resp.ReGeocode.Pois {
			arr := strings.Split(poi.Location, ",")
			loc := ""
			if len(arr) == 2 {
				loc = fmt.Sprintf("%s,%s", arr[1], arr[0])
			}
			res.Pois = append(res.Pois, PoiSmartItem{
				Place:    poi.Name,
				Address:  poi.Address,
				Location: loc,
			})
		}
	} else {
		//Road
		if len(resp.ReGeocode.Roads) > 0 {
			for _, road := range resp.ReGeocode.Roads {
				arr := strings.Split(road.Location, ",")
				loc := ""
				if len(arr) == 2 {
					loc = fmt.Sprintf("%s,%s", arr[1], arr[0])
				}
				res.Pois = append(res.Pois, PoiSmartItem{
					Place:    road.Name,
					Address:  "",
					Location: loc,
				})
			}
		}
	}

	return
}

// ReGeoSmart location "纬,经"
func ReGeoSmart(key, location string) (res ReGeoSmartRes, err error) {
	//解析
	resp, err := _ReGeoSmart(key, location)
	if err != nil {
		log.Println(err)
		return
	}
	res.FormattedAddress = resp.ReGeocode.FormattedAddress
	res.Province = resp.ReGeocode.AddressComponent.Province
	res.City = resp.ReGeocode.AddressComponent.City
	res.District = resp.ReGeocode.AddressComponent.District
	res.Township = resp.ReGeocode.AddressComponent.Township

	//POI
	if len(resp.ReGeocode.Pois) > 0 {
		poi := resp.ReGeocode.Pois[0]

		res.Address = poi.Address

		res.Place = poi.Name
		arr := strings.Split(poi.Location, ",")
		if len(arr) == 2 {
			res.Location = fmt.Sprintf("%s,%s", arr[1], arr[0])
		}
	} else {
		//Road
		if len(resp.ReGeocode.Roads) > 0 {
			road := resp.ReGeocode.Roads[0]
			res.Place = road.Name
			arr := strings.Split(road.Location, ",")
			if len(arr) == 2 {
				res.Location = fmt.Sprintf("%s,%s", arr[1], arr[0])
			}
		}
	}

	return
}

// ReGeoSmart location "纬,经"
func _ReGeoSmart(key, location string) (resp reGeoSmartRes, err error) {

	arr := strings.Split(location, ",")
	if len(arr) == 2 {
		location = fmt.Sprintf("%s,%s", arr[1], arr[0])
	} else {
		err = fmt.Errorf("location error")
		return
	}

	//location "经,纬"
	resBytes, err := request.HttpGet("https://restapi.amap.com/v3/geocode/regeo", map[string]string{
		"key":        key,
		"location":   location,
		"extensions": "all",
	})
	if err != nil {
		log.Println(err)
		return
	}

	//解析
	err = json.Unmarshal(resBytes, &resp)
	if err != nil {
		log.Println(err)
		return
	}
	if resp.Status != "1" {
		err = errors.New(resp.Info)
		log.Println(resp)
		return
	}

	//修复数据
	if val, ok := resp.ReGeocode.AddressComponent.ICity.(string); ok {
		resp.ReGeocode.AddressComponent.City = val
	}
	if val, ok := resp.ReGeocode.AddressComponent.IDistrict.(string); ok {
		resp.ReGeocode.AddressComponent.District = val
	}
	if val, ok := resp.ReGeocode.AddressComponent.ITownship.(string); ok {
		resp.ReGeocode.AddressComponent.Township = val
	}
	for i, poi := range resp.ReGeocode.Pois {
		if val, ok := poi.IAddress.(string); ok {
			resp.ReGeocode.Pois[i].Address = val
		}
		if f, fErr := strconv.ParseFloat(poi.ODistance, 64); fErr == nil {
			resp.ReGeocode.Pois[i].Distance = f
		}
	}

	//根据距离排序
	sort.Slice(resp.ReGeocode.Pois, func(i, j int) bool {
		if resp.ReGeocode.Pois[i].Distance < resp.ReGeocode.Pois[j].Distance {
			return true
		}
		return false
	})

	return
}
