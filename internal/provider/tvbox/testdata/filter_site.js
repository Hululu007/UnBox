var rule = {
  host: "https://example.com",
  url: "/list/fyclass-fypage.html",
  filter_url: "&area={{fl.area}}",
  filter_def: {"1": {"area": {"name": "地区", "options": [{"n": "全部", "v": ""}, {"n": "欧美", "v": "欧美"}], "value": ""}}},
  class_name: "电影",
  class_url: "1",
  vod_selector: ".item",
  id_selector: "a",
  name_selector: "h3",
};
function categoryContent(tid, pg, filter, extend) {
  var area = extend && extend.area ? extend.area : "默认";
  return JSON.stringify({list: [{vod_id: "1", vod_name: "热片", type_name: area}]});
}
