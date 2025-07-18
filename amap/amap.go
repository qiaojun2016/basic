package amap

import (
	"github.com/qiaojun2016/basic/amap/direction"
	"github.com/qiaojun2016/basic/amap/geocode"
	regeo2 "github.com/qiaojun2016/basic/amap/regeo"
	"github.com/qiaojun2016/basic/amap/search"
	"github.com/qiaojun2016/basic/color"
)

//正规表示发:lat,lon或者[lon,lat]
//纬度,经度 或者 [经度,纬度]

var Amap *server

type (
	Server struct {
		WebKey string
	}

	server struct {
		server Server
	}
)

//Geo 地理编码 API 服务地址，名称转坐标
func (s server) Geo(address string) (res geocode.GeoRes, err error) {
	return geocode.Geo(s.server.WebKey, address)
}

//ReGeo 地理编码 API 服务地址，坐标转名称
func (s server) ReGeo(location string) (geocodes regeo2.ReGeocode, err error) {
	return regeo2.ReGeo(s.server.WebKey, location)
}

//ReGeoSmart 地理编码 API 服务地址，坐标转名称
func (s server) ReGeoSmart(location string) (res regeo2.ReGeoSmartRes, err error) {
	return regeo2.ReGeoSmart(s.server.WebKey, location)
}

//ReGeoSmartList 地理编码 API 服务地址，坐标转名称
func (s server) ReGeoSmartList(location string) (res regeo2.ReGeoSmartListRes, err error) {
	return regeo2.ReGeoSmartList(s.server.WebKey, location)
}

//Search 搜索
func (s server) Search(keywords, types, region string) (res []search.SearchPoi, err error) {
	return search.Search(s.server.WebKey, keywords, types, region)
}

//Detail 根据AOI或POI的id查询
func (s server) Detail(id string) (res search.DetailPoiRes, err error) {
	return search.Detail(s.server.WebKey, id)
}

//Driving 行车规划基础信息
func (s server) Driving(region, destination string) (res direction.DrivingResp, err error) {
	return direction.Driving(s.server.WebKey, region, destination)
}

//DrivingPolyline 行车规划polyline
func (s server) DrivingPolyline(region, destination string) (res direction.DrivingPolylines, err error) {
	return direction.DrivingPolyline(s.server.WebKey, region, destination)
}

//DrivingPointsPolyline 返回一串坐标的Polyline,[points]参数用,和;隔开
func (s server) DrivingPointsPolyline(points string) (res direction.DrivingPolylines, err error) {
	return direction.DrivingPointsPolyline(s.server.WebKey, points)
}

//ReGeoContains 返回一串坐标的Polyline,[points]参数用,和;隔开
func (s server) ReGeoContains(location string, address []string) (res bool, err error) {
	return regeo2.ReGeoContains(s.server.WebKey, location, address)
}

func (s Server) Run() {
	//防止多次创建
	if Amap != nil {
		return
	}
	//创建对象
	Amap = &server{server: s}
	color.Success("[amap] create client success")
}
