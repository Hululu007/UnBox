// cat.js — UnBox 自有 shim（不使用上游 FongMi/TV 的 GPL-3.0 实现）。
//
// 只覆盖真实站点脚本用到的能力：
//   Crypto  —— 直接复用内置的 MIT crypto-js 命名空间；
//   _       —— lodash 常用子集（真实脚本里常见 floor/isEmpty/sortBy/get/map…）；
//   jinja2  —— 轻量模板渲染（{{ }} 插值、{% if %}、{% for %}），
//              覆盖 drpy 过滤参数与页面模板的常见写法。
//
// 未实现的 lodash/jinja2 高级特性会显式返回 undefined 或原样保留，不会静默伪造结果。
import CryptoJS from "assets://js/lib/crypto-js.js";

export const Crypto = CryptoJS;

function isPlainObject(value) {
	return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isEmpty(value) {
	if (value === null || value === undefined) return true;
	if (typeof value === "string" || Array.isArray(value)) return value.length === 0;
	if (isPlainObject(value)) return Object.keys(value).length === 0;
	return false;
}

function toPath(path) {
	if (Array.isArray(path)) return path;
	return String(path)
		.replace(/\[(\d+)\]/g, ".$1")
		.split(".")
		.filter(function (segment) {
			return segment !== "";
		});
}

function get(object, path, defaultValue) {
	let current = object;
	const segments = toPath(path);
	for (let i = 0; i < segments.length; i++) {
		if (current === null || current === undefined) {
			return defaultValue;
		}
		current = current[segments[i]];
	}
	return current === undefined ? defaultValue : current;
}

function sortBy(list, iteratee) {
	const key =
		typeof iteratee === "function"
			? iteratee
			: function (item) {
					return get(item, iteratee);
			  };
	return (Array.isArray(list) ? list.slice() : []).sort(function (a, b) {
		const va = key(a);
		const vb = key(b);
		if (va === vb) return 0;
		return va < vb ? -1 : 1;
	});
}

function uniqBy(list, iteratee) {
	const key =
		typeof iteratee === "function"
			? iteratee
			: function (item) {
					return get(item, iteratee);
			  };
	const seen = new Set();
	const out = [];
	(Array.isArray(list) ? list : []).forEach(function (item) {
		const k = key(item);
		if (seen.has(k)) return;
		seen.add(k);
		out.push(item);
	});
	return out;
}

function flattenDeep(list) {
	const out = [];
	(function walk(items) {
		(Array.isArray(items) ? items : [items]).forEach(function (item) {
			if (Array.isArray(item)) walk(item);
			else out.push(item);
		});
	})(list);
	return out;
}

function groupBy(list, iteratee) {
	const key =
		typeof iteratee === "function"
			? iteratee
			: function (item) {
					return get(item, iteratee);
			  };
	const out = {};
	(Array.isArray(list) ? list : []).forEach(function (item) {
		const k = key(item);
		if (!out[k]) out[k] = [];
		out[k].push(item);
	});
	return out;
}

function pick(object, keys) {
	const out = {};
	(Array.isArray(keys) ? keys : []).forEach(function (key) {
		if (object && Object.prototype.hasOwnProperty.call(object, key)) {
			out[key] = object[key];
		}
	});
	return out;
}

function omit(object, keys) {
	const drop = new Set(Array.isArray(keys) ? keys : []);
	const out = {};
	Object.keys(object || {}).forEach(function (key) {
		if (!drop.has(key)) out[key] = object[key];
	});
	return out;
}

export const _ = {
	isArray: Array.isArray,
	isString: function (v) {
		return typeof v === "string";
	},
	isObject: function (v) {
		return v !== null && typeof v === "object";
	},
	isFunction: function (v) {
		return typeof v === "function";
	},
	isEmpty: isEmpty,
	get: get,
	has: function (object, path) {
		return get(object, path, undefined) !== undefined;
	},
	keys: function (object) {
		return Object.keys(object || {});
	},
	values: function (object) {
		return Object.values(object || {});
	},
	map: function (list, iteratee) {
		const key =
			typeof iteratee === "function"
				? iteratee
				: function (item) {
						return get(item, iteratee);
				  };
		return (Array.isArray(list) ? list : []).map(function (item, index) {
			return key(item, index);
		});
	},
	filter: function (list, predicate) {
		const key =
			typeof predicate === "function"
				? predicate
				: function (item) {
						return get(item, predicate);
				  };
		return (Array.isArray(list) ? list : []).filter(function (item, index) {
			return key(item, index);
		});
	},
	find: function (list, predicate) {
		const key =
			typeof predicate === "function"
				? predicate
				: function (item) {
						return get(item, predicate);
				  };
		return (Array.isArray(list) ? list : []).find(function (item, index) {
			return key(item, index);
		});
	},
	includes: function (list, value) {
		if (typeof list === "string") return list.indexOf(value) !== -1;
		return (Array.isArray(list) ? list : []).indexOf(value) !== -1;
	},
	first: function (list) {
		return Array.isArray(list) ? list[0] : undefined;
	},
	last: function (list) {
		return Array.isArray(list) ? list[list.length - 1] : undefined;
	},
	compact: function (list) {
		return (Array.isArray(list) ? list : []).filter(Boolean);
	},
	uniq: function (list) {
		return Array.from(new Set(Array.isArray(list) ? list : []));
	},
	uniqBy: uniqBy,
	flatten: function (list) {
		return flattenDeep(list);
	},
	flattenDeep: flattenDeep,
	chunk: function (list, size) {
		const n = size > 0 ? size : 1;
		const out = [];
		const items = Array.isArray(list) ? list : [];
		for (let i = 0; i < items.length; i += n) out.push(items.slice(i, i + n));
		return out;
	},
	range: function (start, end, step) {
		let from = start;
		let to = end;
		if (to === undefined) {
			to = from;
			from = 0;
		}
		const by = step === undefined ? 1 : step;
		const out = [];
		if (by === 0) return out;
		for (let i = from; by > 0 ? i < to : i > to; i += by) out.push(i);
		return out;
	},
	sum: function (list) {
		return (Array.isArray(list) ? list : []).reduce(function (acc, item) {
			return acc + (Number(item) || 0);
		}, 0);
	},
	maxBy: function (list, iteratee) {
		return sortBy(list, iteratee).pop();
	},
	minBy: function (list, iteratee) {
		return sortBy(list, iteratee).shift();
	},
	sortBy: sortBy,
	orderBy: function (list, iteratees, orders) {
		const keys = Array.isArray(iteratees) ? iteratees : [iteratees];
		const dirs = Array.isArray(orders) ? orders : [orders];
		return (Array.isArray(list) ? list.slice() : []).sort(function (a, b) {
			for (let i = 0; i < keys.length; i++) {
				const va = typeof keys[i] === "function" ? keys[i](a) : get(a, keys[i]);
				const vb = typeof keys[i] === "function" ? keys[i](b) : get(b, keys[i]);
				if (va === vb) continue;
				const desc = String(dirs[i] || "asc").toLowerCase() === "desc";
				return (va < vb ? -1 : 1) * (desc ? -1 : 1);
			}
			return 0;
		});
	},
	groupBy: groupBy,
	countBy: function (list, iteratee) {
		const groups = groupBy(list, iteratee);
		const out = {};
		Object.keys(groups).forEach(function (key) {
			out[key] = groups[key].length;
		});
		return out;
	},
	pick: pick,
	omit: omit,
	cloneDeep: function (value) {
		return value === undefined ? value : JSON.parse(JSON.stringify(value));
	},
	floor: function (value, precision) {
		const p = precision || 0;
		const factor = Math.pow(10, p);
		return Math.floor(Number(value) * factor) / factor;
	},
	ceil: function (value, precision) {
		const p = precision || 0;
		const factor = Math.pow(10, p);
		return Math.ceil(Number(value) * factor) / factor;
	},
	round: function (value, precision) {
		const p = precision || 0;
		const factor = Math.pow(10, p);
		return Math.round(Number(value) * factor) / factor;
	},
	trim: function (value) {
		return String(value === undefined ? "" : value).trim();
	},
	toLower: function (value) {
		return String(value === undefined ? "" : value).toLowerCase();
	},
	toUpper: function (value) {
		return String(value === undefined ? "" : value).toUpperCase();
	},
	capitalize: function (value) {
		const text = String(value === undefined ? "" : value);
		return text.charAt(0).toUpperCase() + text.slice(1).toLowerCase();
	},
	startsWith: function (value, target) {
		return String(value === undefined ? "" : value).indexOf(target) === 0;
	},
	endsWith: function (value, target) {
		const text = String(value === undefined ? "" : value);
		return text.lastIndexOf(target) === text.length - String(target).length;
	},
	repeat: function (value, count) {
		return new Array(Math.max(0, count || 0) + 1).join(String(value));
	},
	padStart: function (value, length, chars) {
		return String(value).padStart(length, chars === undefined ? " " : chars);
	},
	defaultTo: function (value, fallback) {
		return value === null || value === undefined || Number.isNaN(value) ? fallback : value;
	},
	escapeRegExp: function (value) {
		return String(value === undefined ? "" : value).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
	},
	debounce: function (fn) {
		// 同步运行时没有真正的延迟，直接返回原函数。
		return fn;
	},
	throttle: function (fn) {
		return fn;
	},
};

// ---- jinja2 轻量实现 ----

function lookup(context, expression) {
	const path = String(expression).trim();
	if (!path) return undefined;
	if (Object.prototype.hasOwnProperty.call(context, path)) return context[path];
	return get(context, path);
}

function applyFilters(value, filters) {
	let current = value;
	filters.forEach(function (filter) {
		const name = filter.name;
		const arg = filter.arg;
		if (name === "default" || name === "d") {
			if (current === undefined || current === null || current === "") {
				current = arg;
			}
		} else if (name === "trim") {
			current = String(current === undefined ? "" : current).trim();
		} else if (name === "upper" || name === "uppercase") {
			current = String(current === undefined ? "" : current).toUpperCase();
		} else if (name === "lower" || name === "lowercase") {
			current = String(current === undefined ? "" : current).toLowerCase();
		} else if (name === "safe") {
			// 不做 HTML 转义，保持原值。
		} else if (name === "length") {
			current = current === undefined || current === null ? 0 : current.length;
		} else if (name === "first") {
			current = Array.isArray(current) ? current[0] : undefined;
		} else if (name === "last") {
			current = Array.isArray(current) ? current[current.length - 1] : undefined;
		}
	});
	return current;
}

function renderExpression(expression, context) {
	const parts = String(expression).split("|");
	const head = parts.shift();
	let value = lookup(context, head);
	const filters = parts.map(function (raw) {
		const match = /^\s*([A-Za-z_][\w]*)\s*(?:\(\s*(['"]?)(.*?)\2\s*\))?\s*$/.exec(raw);
		if (!match) return { name: "", arg: "" };
		return { name: match[1], arg: match[3] === undefined ? "" : match[3] };
	});
	return applyFilters(value, filters);
}

function stringify(value) {
	if (value === undefined || value === null) return "";
	return String(value);
}

function tokenize(template) {
	return String(template === undefined ? "" : template).split(
		/(\{\{[\s\S]*?\}\}|\{%[\s\S]*?%\})/g,
	);
}

// parseNodes 把 token 序列解析成 [{kind:text|expr|if|for}] 树。
function parseNodes(tokens, start, stops) {
	const nodes = [];
	let i = start;
	while (i < tokens.length) {
		const token = tokens[i];
		if (!token) {
			i++;
			continue;
		}
		if (token.indexOf("{{") === 0) {
			nodes.push({ kind: "expr", expr: token.slice(2, -2) });
			i++;
			continue;
		}
		if (token.indexOf("{%") !== 0) {
			nodes.push({ kind: "text", text: token });
			i++;
			continue;
		}
		const statement = token.slice(2, -2).trim();
		const head = statement.split(/\s+/)[0];
		if (stops.indexOf(head) !== -1) return { nodes: nodes, next: i };
		if (head === "if") {
			const thenPart = parseNodes(tokens, i + 1, ["else", "endif"]);
			let elseNodes = [];
			i = thenPart.next;
			const stop = tokens[i] ? tokens[i].slice(2, -2).trim() : "";
			if (stop === "else") {
				const elsePart = parseNodes(tokens, i + 1, ["endif"]);
				elseNodes = elsePart.nodes;
				i = elsePart.next;
			}
			i++; // 跳过 endif
			nodes.push({
				kind: "if",
				cond: statement.slice(2).trim(),
				then: thenPart.nodes,
				otherwise: elseNodes,
			});
			continue;
		}
		if (head === "for") {
			const match = /^for\s+([A-Za-z_$][\w$]*)\s+in\s+(.+)$/.exec(statement);
			const body = parseNodes(tokens, i + 1, ["endfor"]);
			i = body.next + 1; // 跳过 endfor
			nodes.push({
				kind: "for",
				name: match ? match[1] : "",
				list: match ? match[2].trim() : "",
				body: body.nodes,
			});
			continue;
		}
		// 未知标签（include/set 等）跳过，避免整段模板失败。
		i++;
	}
	return { nodes: nodes, next: i };
}

function renderNodes(nodes, scope) {
	let out = "";
	nodes.forEach(function (node) {
		if (node.kind === "text") {
			out += node.text;
			return;
		}
		if (node.kind === "expr") {
			out += stringify(renderExpression(node.expr, scope));
			return;
		}
		if (node.kind === "if") {
			const hit = Boolean(renderExpression(node.cond, scope));
			out += renderNodes(hit ? node.then : node.otherwise, scope);
			return;
		}
		if (node.kind === "for") {
			const list = lookup(scope, node.list);
			const items = Array.isArray(list) ? list : [];
			items.forEach(function (item, index) {
				const inner = Object.assign({}, scope);
				inner[node.name] = item;
				inner.loop = {
					index: index + 1,
					index0: index,
					first: index === 0,
					last: index === items.length - 1,
				};
				out += renderNodes(node.body, inner);
			});
		}
	});
	return out;
}

export function jinja2(template, context) {
	const ctx = context || {};
	// drpy 的过滤模板约定把筛选值放在 fl 下，这里补齐默认上下文。
	const scope = Object.assign({ fl: ctx.fl || {} }, ctx);
	return renderNodes(parseNodes(tokenize(template), 0, []).nodes, scope);
}
