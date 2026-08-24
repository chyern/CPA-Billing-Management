package dashboard

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Data struct {
	Summary  any `json:"summary,omitempty"`
	Rules    any `json:"rules,omitempty"`
	Currency any `json:"currency,omitempty"`
}

// managementAuthScript mirrors the CLIProxyAPI management center's browser
// credential contract. The panel persists its Zustand auth state under
// cli-proxy-auth and wraps the JSON in enc::v1::<base64 XOR payload>. The
// resource page is served from the same origin, so it can reuse that state
// without receiving a second copy of the management key in its HTML.
const managementAuthScript = `
const AUTH_STORAGE_KEY='cli-proxy-auth'; const AUTH_LOGIN_MARKER='isLoggedIn'; const AUTH_PREFIX='enc::v1::'; const AUTH_SALT='cli-proxy-api-webui::secure-storage';
function readManagementKey(){try{if(localStorage.getItem(AUTH_LOGIN_MARKER)!=='true')return '';const raw=localStorage.getItem(AUTH_STORAGE_KEY);if(!raw)return '';let json=raw;if(raw.startsWith(AUTH_PREFIX)){const binary=atob(raw.slice(AUTH_PREFIX.length));const encrypted=new Uint8Array(binary.length);for(let i=0;i<binary.length;i++)encrypted[i]=binary.charCodeAt(i);const key=new TextEncoder().encode(AUTH_SALT+'|'+window.location.host+'|'+navigator.userAgent);const plain=new Uint8Array(encrypted.length);for(let i=0;i<encrypted.length;i++)plain[i]=encrypted[i]^key[i%key.length];json=new TextDecoder().decode(plain)}const payload=JSON.parse(json);const state=payload&&typeof payload==='object'&&payload.state&&typeof payload.state==='object'?payload.state:payload;return typeof state.managementKey==='string'?state.managementKey.trim():''}catch(_){return ''}}
const ENFORCE_MANAGEMENT_AUTH=window.location.pathname.startsWith('/v0/resource/plugins/'); const MANAGEMENT_KEY=readManagementKey(); const authHeaders=()=>MANAGEMENT_KEY?{Authorization:'Bearer '+MANAGEMENT_KEY}:{};
function redirectToManagementLogin(){const target=new URL('/management.html#/login',window.location.origin).href;try{if(window.top&&window.top!==window){window.top.location.href=target;return}}catch(_){}window.location.replace(target)}
function requireManagementKey(){if(ENFORCE_MANAGEMENT_AUTH&&!MANAGEMENT_KEY){redirectToManagementLogin();return false}return true}
`

const styles = `
:root{color-scheme:light dark;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f5f7fb;color:#182230}
body{margin:0;padding:24px;max-width:1180px;margin-inline:auto}.header{display:flex;justify-content:space-between;gap:16px;align-items:flex-start;margin-bottom:20px}h1{margin:0 0 6px;font-size:26px}h2{font-size:18px;margin:0 0 16px}.muted{color:#6b7280;font-size:13px}.toolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.toolbar select{border:1px solid #cbd5e1;border-radius:8px;padding:7px 28px 7px 10px;background:#fff}.grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;margin-bottom:20px}.card,.panel{background:#fff;border:1px solid #e5e7eb;border-radius:12px;padding:16px;box-shadow:0 2px 8px #0000000a}.card .label{font-size:13px;color:#6b7280}.card .value{font-size:25px;font-weight:700;margin-top:8px}.panel{margin-bottom:20px;overflow:auto}table{width:100%;border-collapse:collapse;min-width:720px}th,td{text-align:left;padding:10px 8px;border-bottom:1px solid #e5e7eb;font-size:13px;white-space:nowrap}th{color:#6b7280;font-weight:600}td.num{text-align:right;font-variant-numeric:tabular-nums}th.num{text-align:right}.pill{display:inline-block;padding:3px 8px;border-radius:999px;background:#fef3c7;color:#92400e;font-size:12px}.actions{display:flex;gap:8px;margin-top:14px}.pager{display:flex;align-items:center;justify-content:center;gap:12px;padding-top:14px}.btn{border:1px solid #cbd5e1;border-radius:8px;padding:7px 12px;cursor:pointer;background:#fff}.btn:disabled{cursor:not-allowed;opacity:.45}.btn.primary{background:#2563eb;color:#fff;border-color:#2563eb}.btn.danger{color:#b91c1c}.rules input{width:120px;padding:6px;border:1px solid #cbd5e1;border-radius:6px}.rules input.match{width:190px}.rules td{padding-block:6px}.empty{text-align:center;color:#6b7280;padding:26px}.status{min-height:18px}.status.error{color:#b91c1c}.footer{font-size:12px;color:#6b7280;text-align:center;padding:8px}
@media(prefers-color-scheme:dark){:root{background:#111827;color:#e5e7eb}.card,.panel{background:#1f2937;border-color:#374151}.card .label,.muted,.footer,th{color:#9ca3af}th,td{border-color:#374151}input,select,.btn,.toolbar select{background:#111827;color:#e5e7eb;border-color:#4b5563}.btn.primary{background:#2563eb;color:#fff;border-color:#2563eb}.btn.danger,.status.error{color:#fca5a5}}
@media(max-width:760px){body{padding:14px}.header{flex-direction:column}.toolbar{width:100%}.grid{grid-template-columns:repeat(2,minmax(0,1fr))}.card .value{font-size:20px}}
`

func RenderBilling(data Data) ([]byte, error) {
	initial, err := initialJSON(data)
	if err != nil {
		return nil, err
	}
	page := `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>CPA 费用统计</title><style>` + styles + `</style></head><body>
<div class="header"><div><h1>CPA 费用统计</h1><div class="muted">上游明确返回金额时优先使用，否则按“模型费用”中的每百万 token 价格估算</div></div><div class="toolbar"><span class="muted">自动刷新</span><select id="autoRefresh" aria-label="自动刷新"><option value="0">不刷新</option><option value="5">5 秒</option><option value="10">10 秒</option><option value="15" selected>15 秒</option></select><span id="status" class="muted status"></span></div></div>
<section id="cards" class="grid"></section>
<section class="panel"><h2>按模型汇总</h2><div id="models"></div></section>
<section class="panel"><h2>按 API Key 汇总</h2><div id="apiKeys"></div></section>
<section class="panel"><h2>最近事件</h2><div id="events"></div></section>
<div class="footer">CPA Billing Management · 模型费用为估算值，请以供应商账单为准</div>
<script id="initial" type="application/json">` + initial + `</script>
	<script>` + managementAuthScript + `
	const BASE='/v0/management/cpa-billing-management/summary'; const PAGE_SIZE=20; let refreshTimer=null;
let state=JSON.parse(document.getElementById('initial').textContent).summary||{currency:'USD',totals:{},models:[],api_keys:[],recent_events:[],recent_events_total:0,recent_events_page:1,recent_events_pages:1,recent_events_page_size:PAGE_SIZE};
const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const n=x=>Number(x||0).toLocaleString('zh-CN'); const money=x=>esc(state.currency||'USD')+' '+Number(x||0).toFixed(6);
const duration=ns=>{const ms=Number(ns||0)/1e6;if(!Number.isFinite(ms)||ms<=0)return '-';if(ms<1)return '不足 1 ms';if(ms<1000)return Math.round(ms)+' ms';if(ms<10000)return (ms/1000).toFixed(2)+' s';return (ms/1000).toFixed(1)+' s'};
function render(){const t=state.totals||{};document.getElementById('cards').innerHTML=[['总费用',money(t.cost)],['请求数',n(t.requests)],['总 token',n(t.total_tokens)],['失败请求',n(t.failed_requests)]].map(x=>'<div class="card"><div class="label">'+x[0]+'</div><div class="value">'+x[1]+'</div></div>').join('');
const models=state.models||[];document.getElementById('models').innerHTML=models.length?'<table><thead><tr><th>Provider</th><th>Model</th><th class="num">请求</th><th class="num">输入</th><th class="num">缓存</th><th class="num">输出</th><th class="num">总 token</th><th class="num">费用</th></tr></thead><tbody>'+models.map(m=>'<tr><td>'+esc(m.provider)+'</td><td>'+esc(m.model)+' '+(m.priced?'':'<span class="pill">未配置模型费用</span>')+'</td><td class="num">'+n(m.requests)+'</td><td class="num">'+n(m.input_tokens)+'</td><td class="num">'+n(m.cached_tokens)+'</td><td class="num">'+n(m.output_tokens)+'</td><td class="num">'+n(m.total_tokens)+'</td><td class="num">'+money(m.cost)+'</td></tr>').join('')+'</tbody></table>':'<div class="empty">暂无 usage 事件</div>';
const apiKeys=state.api_keys||[];document.getElementById('apiKeys').innerHTML=apiKeys.length?'<table><thead><tr><th>API Key</th><th class="num">请求</th><th class="num">失败</th><th class="num">输入</th><th class="num">缓存</th><th class="num">输出</th><th class="num">总 token</th><th class="num">费用</th></tr></thead><tbody>'+apiKeys.map(k=>'<tr><td>'+esc(k.api_key||'未提供')+'</td><td class="num">'+n(k.requests)+'</td><td class="num">'+n(k.failed_requests)+'</td><td class="num">'+n(k.input_tokens)+'</td><td class="num">'+n(k.cached_tokens)+'</td><td class="num">'+n(k.output_tokens)+'</td><td class="num">'+n(k.total_tokens)+'</td><td class="num">'+money(k.cost)+'</td></tr>').join('')+'</tbody></table>':'<div class="empty">暂无 API Key 数据</div>';
const eventTable=(state.recent_events||[]).length?'<table><thead><tr><th>时间</th><th>模型</th><th>API Key</th><th class="num">耗时/首字</th><th class="num">输入</th><th class="num">缓存</th><th class="num">输出</th><th class="num">费用</th><th>状态</th></tr></thead><tbody>'+state.recent_events.slice().reverse().map(e=>'<tr><td>'+esc(new Date(e.requested_at).toLocaleString())+'</td><td>'+esc(e.model||'-')+'</td><td>'+esc(e.api_key||'-')+'</td><td class="num">'+duration(e.latency_ns)+' / '+duration(e.ttft_ns)+'</td><td class="num">'+n(e.input_tokens)+'</td><td class="num">'+n(e.cached_tokens)+'</td><td class="num">'+n(e.output_tokens)+'</td><td class="num">'+money(e.cost)+'</td><td>'+(e.failed?'<span class="pill">失败</span>':'成功')+'</td></tr>').join('')+'</tbody></table>':'<div class="empty">暂无最近事件</div>'; const page=Number(state.recent_events_page||1), pages=Math.max(1,Number(state.recent_events_pages||1)), total=Number(state.recent_events_total||0); document.getElementById('events').innerHTML=eventTable+'<div class="pager"><button class="btn" id="prevPage" '+(page<=1?'disabled':'')+'>上一页</button><span class="muted">第 '+page+' / '+pages+' 页 · 共 '+n(total)+' 条</span><button class="btn" id="nextPage" '+(page>=pages?'disabled':'')+'>下一页</button></div>'; document.getElementById('prevPage').onclick=()=>loadPage(page-1); document.getElementById('nextPage').onclick=()=>loadPage(page+1)}
function status(msg,error){const el=document.getElementById('status');el.textContent=msg;el.className='muted status'+(error?' error':'')}
	async function loadPage(page){if(!requireManagementKey())return;const pageNumber=Math.max(1,page);try{const res=await fetch(BASE+'?page='+pageNumber+'&page_size='+PAGE_SIZE,{credentials:'same-origin',headers:authHeaders()});if(res.status===401){redirectToManagementLogin();return}if(!res.ok)throw new Error(await res.text()||res.statusText);const payload=await res.json();state=payload.summary||payload;render();status('已更新')}catch(error){status('更新失败：'+(error.message||'请求失败'),true)}}
const autoRefresh=document.getElementById('autoRefresh');
function configureAutoRefresh(seconds){if(refreshTimer)clearInterval(refreshTimer);refreshTimer=null;if(seconds>0)refreshTimer=setInterval(()=>loadPage(Number(state.recent_events_page||1)),seconds*1000)}
autoRefresh.onchange=e=>configureAutoRefresh(Number(e.target.value));
configureAutoRefresh(Number(autoRefresh.value)); render();
loadPage(Number(state.recent_events_page||1));
</script></body></html>`
	return []byte(page), nil
}

func RenderPricing(data Data) ([]byte, error) {
	initial, err := initialJSON(data)
	if err != nil {
		return nil, err
	}
	page := `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>CPA 模型费用</title><style>` + styles + `</style></head><body>
<div class="header"><div><h1>CPA 模型费用</h1><div class="muted">配置模型每百万 token 的估算价格；上游明确返回金额时不会被这里的规则覆盖</div></div><div class="toolbar"><span id="status" class="muted status"></span></div></div>
<section class="panel"><h2>模型价格规则</h2><p class="muted">匹配优先级：provider/model → model → alias → *。价格单位为当前币种 / 1M token，保存后会重新计算无上游金额的历史事件。</p><div id="rules" class="rules"></div><div class="actions"><button class="btn" id="sync">同步上游价格</button><button class="btn" id="add">新增规则</button><button class="btn primary" id="save">保存模型费用</button></div></section>
<div class="footer">CPA Billing Management · 当前币种：<span id="currency"></span> · 估算费用仅供参考</div>
<script id="initial" type="application/json">` + initial + `</script>
	<script>` + managementAuthScript + `
	const BASE='/v0/management/cpa-billing-management/prices'; const SYNC_BASE=BASE+'/sync';
	const initial=JSON.parse(document.getElementById('initial').textContent); let rules=initial.rules||[];document.getElementById('currency').textContent=initial.currency||'USD';
const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const priceFields=['input_per_million','output_per_million','cache_read_per_million','cache_creation_per_million'];
const valueFor=(rule,key)=>rule._draft||rule[key]==null||!Number.isFinite(Number(rule[key]))?'':Number(rule[key]);
function render(){document.getElementById('rules').innerHTML='<table><thead><tr><th>匹配</th><th class="num">输入 / 1M</th><th class="num">输出 / 1M</th><th class="num">缓存读取 / 1M</th><th class="num">缓存创建 / 1M</th><th></th></tr></thead><tbody>'+rules.map((r,i)=>'<tr data-i="'+i+'"><td><input class="match" data-k="match" placeholder="例如：openai/gpt-4o" value="'+(r._draft?'':esc(r.match))+'"></td><td><input data-k="input_per_million" type="number" min="0" step="0.000001" placeholder="例如：2.5" value="'+valueFor(r,'input_per_million')+'"></td><td><input data-k="output_per_million" type="number" min="0" step="0.000001" placeholder="例如：10" value="'+valueFor(r,'output_per_million')+'"></td><td><input data-k="cache_read_per_million" type="number" min="0" step="0.000001" placeholder="例如：0.25" value="'+valueFor(r,'cache_read_per_million')+'"></td><td><input data-k="cache_creation_per_million" type="number" min="0" step="0.000001" placeholder="例如：0.25" value="'+valueFor(r,'cache_creation_per_million')+'"></td><td><button class="btn danger" onclick="removeRule('+i+')">删除</button></td></tr>').join('')+'</tbody></table>'}
window.removeRule=i=>{rules.splice(i,1);render()};
function readRules(){document.querySelectorAll('#rules tbody tr').forEach(row=>{const i=Number(row.dataset.i);row.querySelectorAll('input').forEach(input=>{const k=input.dataset.k;const value=input.value.trim();rules[i][k]=k==='match'?value:(value===''?NaN:Number(value))});delete rules[i]._draft})}
function validateRules(){const seen=new Set();for(let i=0;i<rules.length;i++){const rule=rules[i],match=String(rule.match||'').trim(),key=match.toLowerCase();if(!match)return '第 '+(i+1)+' 条规则的匹配不能为空';if(seen.has(key))return '第 '+(i+1)+' 条规则与其他规则重复：'+match;seen.add(key);for(const field of priceFields){if(!Number.isFinite(rule[field])||rule[field]<0)return '第 '+(i+1)+' 条规则的价格必须是大于等于 0 的有效数字'}}return ''}
function status(msg,error){const el=document.getElementById('status');el.textContent=msg;el.className='muted status'+(error?' error':'')}
	async function api(url=BASE,opts={}){if(!requireManagementKey())throw new Error('管理中心登录已失效');const request=Object.assign({credentials:'same-origin',headers:{'Content-Type':'application/json',...authHeaders()}},opts);const res=await fetch(url,request);if(res.status===401){redirectToManagementLogin();throw new Error('管理中心登录已失效')}if(!res.ok)throw new Error(await res.text()||res.statusText);return res.json()}
document.getElementById('sync').onclick=async()=>{try{const data=await api(SYNC_BASE,{method:'POST'});rules=data.rules||rules;render();status('已同步 '+Number(data.matched||0)+' 个模型')}catch(e){status('同步失败：'+e.message,true)}};
document.getElementById('add').onclick=()=>{readRules();rules.push({_draft:true,match:'',input_per_million:null,output_per_million:null,cache_read_per_million:null,cache_creation_per_million:null});render()};
document.getElementById('save').onclick=async()=>{try{readRules();const validationError=validateRules();if(validationError){status('保存失败：'+validationError,true);return}const data=await api(BASE,{method:'PUT',body:JSON.stringify({rules})});rules=data.rules||rules;render();status('模型费用已保存，历史估算已更新')}catch(e){status('保存失败：'+e.message,true)}};render();
api().then(data=>{rules=data.rules||[];document.getElementById('currency').textContent=data.currency||'USD';render();status('已更新')}).catch(e=>status('加载失败：'+e.message,true));
</script></body></html>`
	return []byte(page), nil
}

// Render is kept as the billing-page entry point for preview integrations.
func Render(data Data) ([]byte, error) { return RenderBilling(data) }

func initialJSON(data Data) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("encode dashboard data: %w", err)
	}
	// JSON is embedded in a script element. Escape HTML-significant bytes so a
	// model name cannot terminate the element and inject markup.
	return strings.NewReplacer("<", "\\u003c", ">", "\\u003e", "&", "\\u0026").Replace(string(raw)), nil
}
