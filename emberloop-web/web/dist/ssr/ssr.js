var __defProp = Object.defineProperty;
var __typeError = (msg) => {
  throw TypeError(msg);
};
var __defNormalProp = (obj, key, value) => key in obj ? __defProp(obj, key, { enumerable: true, configurable: true, writable: true, value }) : obj[key] = value;
var __publicField = (obj, key, value) => __defNormalProp(obj, typeof key !== "symbol" ? key + "" : key, value);
var __accessCheck = (obj, member, msg) => member.has(obj) || __typeError("Cannot " + msg);
var __privateGet = (obj, member, getter) => (__accessCheck(obj, member, "read from private field"), getter ? getter.call(obj) : member.get(obj));
var __privateAdd = (obj, member, value) => member.has(obj) ? __typeError("Cannot add the same private member more than once") : member instanceof WeakSet ? member.add(obj) : member.set(obj, value);
var __privateSet = (obj, member, value, setter) => (__accessCheck(obj, member, "write to private field"), setter ? setter.call(obj, value) : member.set(obj, value), value);
var __privateMethod = (obj, member, method) => (__accessCheck(obj, member, "access private method"), method);
var _render, _render_async, _sync, _hashes, _promise, _RenderResult_instances, get_fn, _out, _on_destroy, _is_component_body, _boundary, _parent, _Renderer_static, serialize_failed_boundary_fn, _Renderer_instances, collect_on_destroy_fn, traverse_components_fn, collect_ondestroy_fn, run_on_destroy_fn, create_fn, render_fn, render_async_fn, collect_content_fn, collect_content_async_fn, collect_hydratables_fn, open_render_fn, close_render_fn, hydratable_block_fn, _pending, _controller, _aborted, _title;
const HYDRATION_START = "[";
const HYDRATION_START_FAILED = "[?";
const HYDRATION_END = "]";
const ELEMENT_IS_NAMESPACED = 1;
const ELEMENT_PRESERVE_ATTRIBUTE_CASE = 1 << 1;
const ELEMENT_IS_INPUT = 1 << 2;
const UNINITIALIZED = Symbol("uninitialized");
const ATTR_REGEX = /[&"<]/g;
const CONTENT_REGEX = /[&<]/g;
function escape_html(value, is_attr) {
  const str = String(value ?? "");
  const pattern = is_attr ? ATTR_REGEX : CONTENT_REGEX;
  pattern.lastIndex = 0;
  let escaped2 = "";
  let last = 0;
  while (pattern.test(str)) {
    const i = pattern.lastIndex - 1;
    const ch = str[i];
    escaped2 += str.substring(last, i) + (ch === "&" ? "&amp;" : ch === '"' ? "&quot;" : "&lt;");
    last = i + 1;
  }
  return escaped2 + str.substring(last);
}
function r(e) {
  var t, f, n = "";
  if ("string" == typeof e || "number" == typeof e) n += e;
  else if ("object" == typeof e) if (Array.isArray(e)) {
    var o = e.length;
    for (t = 0; t < o; t++) e[t] && (f = r(e[t])) && (n && (n += " "), n += f);
  } else for (f in e) e[f] && (n && (n += " "), n += f);
  return n;
}
function clsx$1() {
  for (var e, t, f = 0, n = "", o = arguments.length; f < o; f++) (e = arguments[f]) && (t = r(e)) && (n && (n += " "), n += t);
  return n;
}
var is_array = Array.isArray;
var has_own_property = Object.prototype.hasOwnProperty;
const noop = () => {
};
function deferred() {
  var resolve;
  var reject;
  var promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}
const replacements = {
  translate: /* @__PURE__ */ new Map([
    [true, "yes"],
    [false, "no"]
  ])
};
function attr(name, value, is_boolean = false) {
  if (name === "hidden" && value !== "until-found") {
    is_boolean = true;
  }
  if (value == null || is_boolean && !value && value !== "") return "";
  const normalized = has_own_property.call(replacements, name) && replacements[name].get(value) || value;
  const assignment = is_boolean ? `=""` : `="${escape_html(normalized, true)}"`;
  return ` ${name}${assignment}`;
}
function clsx(value) {
  if (typeof value === "object") {
    return clsx$1(value);
  } else {
    return value ?? "";
  }
}
const whitespace = [..." 	\n\r\f \v\uFEFF"];
function to_class(value, hash, directives) {
  var classname = value == null ? "" : "" + value;
  if (hash) {
    classname = classname ? classname + " " + hash : hash;
  }
  if (directives) {
    for (var key of Object.keys(directives)) {
      if (directives[key]) {
        classname = classname ? classname + " " + key : key;
      } else if (classname.length) {
        var len = key.length;
        var a = 0;
        while ((a = classname.indexOf(key, a)) >= 0) {
          var b = a + len;
          if ((a === 0 || whitespace.includes(classname[a - 1])) && (b === classname.length || whitespace.includes(classname[b]))) {
            classname = (a === 0 ? "" : classname.substring(0, a)) + classname.substring(b + 1);
          } else {
            a = b;
          }
        }
      }
    }
  }
  return classname === "" ? null : classname;
}
function append_styles(styles, important = false) {
  var separator = important ? " !important;" : ";";
  var css = "";
  for (var key of Object.keys(styles)) {
    var value = styles[key];
    if (value != null && value !== "") {
      css += " " + key + ": " + value + separator;
    }
  }
  return css;
}
function to_css_name(name) {
  if (name[0] !== "-" || name[1] !== "-") {
    return name.toLowerCase();
  }
  return name;
}
function to_style(value, styles) {
  if (styles) {
    var new_style = "";
    var normal_styles;
    var important_styles;
    if (Array.isArray(styles)) {
      normal_styles = styles[0];
      important_styles = styles[1];
    } else {
      normal_styles = styles;
    }
    if (value) {
      value = String(value).replaceAll(/\/\*.*?\*\//g, "").trim();
      var in_str = false;
      var in_apo = 0;
      var in_comment = false;
      var reserved_names = [];
      if (normal_styles) {
        reserved_names.push(...Object.keys(normal_styles).map(to_css_name));
      }
      if (important_styles) {
        reserved_names.push(...Object.keys(important_styles).map(to_css_name));
      }
      var start_index = 0;
      var name_index = -1;
      const len = value.length;
      for (var i = 0; i < len; i++) {
        var c = value[i];
        if (in_comment) {
          if (c === "/" && value[i - 1] === "*") {
            in_comment = false;
          }
        } else if (in_str) {
          if (in_str === c) {
            in_str = false;
          }
        } else if (c === "/" && value[i + 1] === "*") {
          in_comment = true;
        } else if (c === '"' || c === "'") {
          in_str = c;
        } else if (c === "(") {
          in_apo++;
        } else if (c === ")") {
          in_apo--;
        }
        if (!in_comment && in_str === false && in_apo === 0) {
          if (c === ":" && name_index === -1) {
            name_index = i;
          } else if (c === ";" || i === len - 1) {
            if (name_index !== -1) {
              var name = to_css_name(value.substring(start_index, name_index).trim());
              if (!reserved_names.includes(name)) {
                if (c !== ";") {
                  i++;
                }
                var property = value.substring(start_index, i).trim();
                new_style += " " + property + ";";
              }
            }
            start_index = i + 1;
            name_index = -1;
          }
        }
      }
    }
    if (normal_styles) {
      new_style += append_styles(normal_styles);
    }
    if (important_styles) {
      new_style += append_styles(important_styles, true);
    }
    new_style = new_style.trim();
    return new_style === "" ? null : new_style;
  }
  return value == null ? null : String(value);
}
const STALE_REACTION = new class StaleReactionError extends Error {
  constructor() {
    super(...arguments);
    __publicField(this, "name", "StaleReactionError");
    __publicField(this, "message", "The reaction that called `getAbortSignal()` was re-run or destroyed");
  }
}();
const BLOCK_OPEN = `<!--${HYDRATION_START}-->`;
const BLOCK_CLOSE = `<!--${HYDRATION_END}-->`;
const DOM_BOOLEAN_ATTRIBUTES = [
  "allowfullscreen",
  "async",
  "autofocus",
  "autoplay",
  "checked",
  "controls",
  "default",
  "disabled",
  "formnovalidate",
  "indeterminate",
  "inert",
  "ismap",
  "loop",
  "multiple",
  "muted",
  "nomodule",
  "novalidate",
  "open",
  "playsinline",
  "readonly",
  "required",
  "reversed",
  "seamless",
  "selected",
  "webkitdirectory",
  "defer",
  "disablepictureinpicture",
  "disableremoteplayback"
];
function is_boolean_attribute(name) {
  return DOM_BOOLEAN_ATTRIBUTES.includes(name);
}
function async_local_storage_unavailable() {
  const error = new Error(`async_local_storage_unavailable
The node API \`AsyncLocalStorage\` is not available, but is required to use async server rendering.
https://svelte.dev/e/async_local_storage_unavailable`);
  error.name = "Svelte error";
  throw error;
}
function await_invalid() {
  const error = new Error(`await_invalid
Encountered asynchronous work while rendering synchronously.
https://svelte.dev/e/await_invalid`);
  error.name = "Svelte error";
  throw error;
}
function invalid_csp() {
  const error = new Error(`invalid_csp
\`csp.nonce\` was set while \`csp.hash\` was \`true\`. These options cannot be used simultaneously.
https://svelte.dev/e/invalid_csp`);
  error.name = "Svelte error";
  throw error;
}
function invalid_id_prefix() {
  const error = new Error(`invalid_id_prefix
The \`idPrefix\` option cannot include \`--\`.
https://svelte.dev/e/invalid_id_prefix`);
  error.name = "Svelte error";
  throw error;
}
function server_context_required() {
  const error = new Error(`server_context_required
Could not resolve \`render\` context.
https://svelte.dev/e/server_context_required`);
  error.name = "Svelte error";
  throw error;
}
var ssr_context = null;
function set_ssr_context(v) {
  ssr_context = v;
}
function push(fn) {
  ssr_context = { p: ssr_context, c: null, r: null, i: false };
}
function pop() {
  ssr_context = /** @type {SSRContext} */
  ssr_context.p;
}
function unresolved_hydratable(key, stack) {
  {
    console.warn(`https://svelte.dev/e/unresolved_hydratable`);
  }
}
let current_render = null;
let context = null;
function get_render_context() {
  const store = context ?? (als == null ? void 0 : als.getStore());
  if (!store) {
    server_context_required();
  }
  return store;
}
async function with_render_context(fn) {
  context = {
    hydratable: {
      lookup: /* @__PURE__ */ new Map(),
      comparisons: [],
      unresolved_promises: /* @__PURE__ */ new Map()
    }
  };
  if (in_webcontainer()) {
    const { promise, resolve } = deferred();
    const previous_render = current_render;
    current_render = promise;
    await previous_render;
    return fn().finally(resolve);
  }
  try {
    if (als === null) {
      async_local_storage_unavailable();
    }
    return als.run(context, fn);
  } finally {
    context = null;
  }
}
let als = null;
let als_import = null;
function init_render_context() {
  als_import ?? (als_import = import("node:async_hooks").then((hooks) => {
    als = new hooks.AsyncLocalStorage();
  }).then(noop, noop));
  return als_import;
}
function in_webcontainer() {
  var _a, _b;
  return !!((_b = (_a = globalThis.process) == null ? void 0 : _a.versions) == null ? void 0 : _b.webcontainer);
}
let text_encoder;
let crypto;
const obfuscated_import = (module_name) => import(
  /* @vite-ignore */
  module_name
);
async function sha256(data) {
  var _a, _b;
  text_encoder ?? (text_encoder = new TextEncoder());
  crypto ?? (crypto = ((_b = (_a = globalThis.crypto) == null ? void 0 : _a.subtle) == null ? void 0 : _b.digest) ? globalThis.crypto : (
    // @ts-ignore - we don't install node types in the prod build
    // don't use import('node:crypto') directly because static analysers will think we rely on node when we don't
    (await obfuscated_import("node:crypto")).webcrypto
  ));
  const hash_buffer = await crypto.subtle.digest("SHA-256", text_encoder.encode(data));
  return base64_encode(hash_buffer);
}
function base64_encode(bytes) {
  if (globalThis.Buffer) {
    return globalThis.Buffer.from(bytes).toString("base64");
  }
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}
const MAX_ARRAY_LEN = 2 ** 32 - 1;
const MAX_ARRAY_INDEX = MAX_ARRAY_LEN - 1;
const escaped = {
  "<": "\\u003C",
  "\\": "\\\\",
  "\b": "\\b",
  "\f": "\\f",
  "\n": "\\n",
  "\r": "\\r",
  "	": "\\t",
  "\u2028": "\\u2028",
  "\u2029": "\\u2029"
};
class DevalueError extends Error {
  /**
   * @param {string} message
   * @param {string[]} keys
   * @param {any} [value] - The value that failed to be serialized
   * @param {any} [root] - The root value being serialized
   */
  constructor(message, keys, value, root) {
    super(message);
    this.name = "DevalueError";
    this.path = keys.join("");
    this.value = value;
    this.root = root;
  }
}
function is_primitive(thing) {
  return thing === null || typeof thing !== "object" && typeof thing !== "function";
}
const object_proto_names = /* @__PURE__ */ Object.getOwnPropertyNames(Object.prototype).sort().join("\0");
function is_plain_object(thing) {
  const proto = Object.getPrototypeOf(thing);
  return proto === Object.prototype || proto === null || Object.getPrototypeOf(proto) === null || Object.getOwnPropertyNames(proto).sort().join("\0") === object_proto_names;
}
function get_type(thing) {
  return Object.prototype.toString.call(thing).slice(8, -1);
}
function is_buffer(thing) {
  return typeof Buffer !== "undefined" && Buffer.isBuffer(thing);
}
function get_escaped_char(char) {
  switch (char) {
    case '"':
      return '\\"';
    case "<":
      return "\\u003C";
    case "\\":
      return "\\\\";
    case "\n":
      return "\\n";
    case "\r":
      return "\\r";
    case "	":
      return "\\t";
    case "\b":
      return "\\b";
    case "\f":
      return "\\f";
    case "\u2028":
      return "\\u2028";
    case "\u2029":
      return "\\u2029";
    default:
      return char < " " ? `\\u${char.charCodeAt(0).toString(16).padStart(4, "0")}` : "";
  }
}
function stringify_string(str) {
  let result = "";
  let last_pos = 0;
  const len = str.length;
  for (let i = 0; i < len; i += 1) {
    const char = str[i];
    const replacement = get_escaped_char(char);
    if (replacement) {
      result += str.slice(last_pos, i) + replacement;
      last_pos = i + 1;
    }
  }
  return `"${last_pos === 0 ? str : result + str.slice(last_pos)}"`;
}
function enumerable_symbols(object) {
  return Object.getOwnPropertySymbols(object).filter(
    (symbol) => Object.getOwnPropertyDescriptor(object, symbol).enumerable
  );
}
const is_identifier = /^[a-zA-Z_$][a-zA-Z_$0-9]*$/;
function stringify_key(key) {
  return is_identifier.test(key) ? "." + key : "[" + JSON.stringify(key) + "]";
}
function is_valid_array_index(n) {
  if (!Number.isInteger(n)) return false;
  if (n < 0) return false;
  if (n > MAX_ARRAY_INDEX) return false;
  return true;
}
function is_valid_array_index_string(s) {
  if (s.length === 0) return false;
  if (s.length > 1 && s.charCodeAt(0) === 48) return false;
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    if (c < 48 || c > 57) return false;
  }
  return is_valid_array_index(+s);
}
function array_index_cut(keys) {
  for (var i = keys.length - 1; i >= 0; i--) {
    if (is_valid_array_index_string(keys[i])) {
      break;
    }
  }
  return i + 1;
}
function valid_array_indices(array) {
  const keys = Object.keys(array);
  keys.length = array_index_cut(keys);
  return keys;
}
const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_$";
const MIN_STRING_LENGTH = 128;
const unsafe_chars = /[<\b\f\n\r\t\0\u2028\u2029]/g;
const reserved = /^(?:do|if|in|for|int|let|new|try|var|byte|case|char|else|enum|goto|long|this|void|with|await|break|catch|class|const|final|float|short|super|throw|while|yield|delete|double|export|import|native|return|switch|throws|typeof|boolean|default|extends|finally|package|private|abstract|continue|debugger|function|volatile|interface|protected|transient|implements|instanceof|synchronized)$/;
function uneval(value, replacer) {
  const counts = /* @__PURE__ */ new Map();
  const keys = [];
  const custom = /* @__PURE__ */ new Map();
  let primitives;
  function stringify_cached_primitive(thing) {
    if (typeof thing === "string" && thing.length >= MIN_STRING_LENGTH || typeof thing === "bigint") {
      primitives ?? (primitives = /* @__PURE__ */ new Map());
      let literal = primitives.get(thing);
      if (literal === void 0) {
        literal = stringify_primitive(thing);
        primitives.set(thing, literal);
      }
      return literal;
    }
    return stringify_primitive(thing);
  }
  function walk(thing) {
    if (!is_primitive(thing)) {
      if (counts.has(thing)) {
        counts.set(thing, counts.get(thing) + 1);
        return;
      }
      counts.set(thing, 1);
      if (typeof thing === "function") {
        throw new DevalueError(`Cannot stringify a function`, keys, thing, value);
      }
      const type = get_type(thing);
      switch (type) {
        case "BigInt":
        case "String":
          walk(thing.valueOf());
          return;
        case "Number":
        case "Boolean":
        case "Date":
        case "RegExp":
        case "URL":
        case "URLSearchParams":
          return;
        case "Array":
          for (const i of valid_array_indices(thing)) {
            keys.push(`[${i}]`);
            walk(thing[i]);
            keys.pop();
          }
          break;
        case "Set":
          Array.from(thing).forEach(walk);
          break;
        case "Map":
          for (const [key, value2] of thing) {
            keys.push(`.get(${is_primitive(key) ? stringify_cached_primitive(key) : "..."})`);
            walk(key);
            walk(value2);
            keys.pop();
          }
          break;
        case "Int8Array":
        case "Uint8Array":
        case "Uint8ClampedArray":
        case "Int16Array":
        case "Uint16Array":
        case "Float16Array":
        case "Int32Array":
        case "Uint32Array":
        case "Float32Array":
        case "Float64Array":
        case "BigInt64Array":
        case "BigUint64Array":
        case "DataView":
          if (!is_buffer(thing)) walk(thing.buffer);
          return;
        case "ArrayBuffer":
          return;
        case "Temporal.Duration":
        case "Temporal.Instant":
        case "Temporal.PlainDate":
        case "Temporal.PlainTime":
        case "Temporal.PlainDateTime":
        case "Temporal.PlainMonthDay":
        case "Temporal.PlainYearMonth":
        case "Temporal.ZonedDateTime":
          return;
        default:
          if (!is_plain_object(thing)) {
            throw new DevalueError(`Cannot stringify arbitrary non-POJOs`, keys, thing, value);
          }
          if (enumerable_symbols(thing).length > 0) {
            throw new DevalueError(`Cannot stringify POJOs with symbolic keys`, keys, thing, value);
          }
          for (const key of Object.keys(thing)) {
            if (key === "__proto__") {
              throw new DevalueError(
                `Cannot stringify objects with __proto__ keys`,
                keys,
                thing,
                value
              );
            }
            keys.push(stringify_key(key));
            walk(thing[key]);
            keys.pop();
          }
      }
    } else if (typeof thing === "symbol") {
      throw new DevalueError(`Cannot stringify a Symbol primitive`, keys, thing, value);
    } else if (typeof thing === "string" && thing.length >= MIN_STRING_LENGTH || typeof thing === "bigint") {
      counts.set(thing, (counts.get(thing) || 0) + 1);
    }
  }
  walk(value);
  const names = /* @__PURE__ */ new Map();
  const repeated = [];
  counts.forEach((count, thing) => {
    if (count > 1) repeated.push([thing, count]);
  });
  repeated.sort((a, b) => b[1] - a[1]).forEach(([thing, count]) => {
    const name = get_name(names.size);
    if (is_primitive(thing)) {
      const length = stringify_cached_primitive(thing).length;
      if (length * count <= length + (count + 1) * name.length + 25) return;
    }
    names.set(thing, name);
  });
  function stringify(thing) {
    if (names.has(thing)) {
      return names.get(thing);
    }
    if (is_primitive(thing)) {
      return stringify_primitive(thing);
    }
    if (custom.has(thing)) {
      return custom.get(thing);
    }
    const type = get_type(thing);
    switch (type) {
      case "Number":
      case "String":
      case "Boolean":
      case "BigInt":
        return `Object(${stringify(thing.valueOf())})`;
      case "RegExp":
        const { source, flags } = thing;
        return flags ? `new RegExp(${stringify_string(source)},"${flags}")` : `new RegExp(${stringify_string(source)})`;
      case "Date":
        return `new Date(${thing.getTime()})`;
      case "URL":
        return `new URL(${stringify_string(thing.toString())})`;
      case "URLSearchParams":
        return `new URLSearchParams(${stringify_string(thing.toString())})`;
      case "Array": {
        let has_holes = false;
        let result = "[";
        for (let i = 0; i < thing.length; i += 1) {
          if (i > 0) result += ",";
          if (Object.hasOwn(thing, i)) {
            result += stringify(thing[i]);
          } else if (!has_holes) {
            const populated_keys = valid_array_indices(thing);
            const population = populated_keys.length;
            const d = String(thing.length).length;
            const array = stringify_sparse_array(thing.length);
            const hole_cost = thing.length + 2;
            const sparse_cost = array.length + 18 + population * (d + 2);
            if (hole_cost > sparse_cost) {
              const entries = populated_keys.map((k) => `${k}:${stringify(thing[k])}`).join(",");
              return `Object.assign(${array},{${entries}})`;
            }
            has_holes = true;
          }
        }
        const tail = thing.length === 0 || Object.hasOwn(thing, thing.length - 1) ? "" : ",";
        return result + tail + "]";
      }
      case "Set":
      case "Map":
        return `new ${type}([${Array.from(thing).map(stringify).join(",")}])`;
      case "Int8Array":
      case "Uint8Array":
      case "Uint8ClampedArray":
      case "Int16Array":
      case "Uint16Array":
      case "Float16Array":
      case "Int32Array":
      case "Uint32Array":
      case "Float32Array":
      case "Float64Array":
      case "BigInt64Array":
      case "BigUint64Array": {
        if (is_buffer(thing)) thing = new Uint8Array(thing);
        let str2 = `new ${type}`;
        if (!names.has(thing.buffer)) {
          str2 += `([${stringify_typed_array_elements(type, thing.buffer)}])`;
        } else {
          str2 += `(${stringify(thing.buffer)})`;
        }
        if (thing.byteLength !== thing.buffer.byteLength) {
          const start = thing.byteOffset / thing.BYTES_PER_ELEMENT;
          const end = start + thing.length;
          str2 += `.subarray(${start},${end})`;
        }
        return str2;
      }
      case "DataView": {
        let str2 = `new DataView`;
        if (!names.has(thing.buffer)) {
          str2 += `(new Uint8Array([${new Uint8Array(thing.buffer)}]).buffer`;
        } else {
          str2 += `(${stringify(thing.buffer)}`;
        }
        if (thing.byteLength !== thing.buffer.byteLength) {
          str2 += `,${thing.byteOffset},${thing.byteLength}`;
        }
        return str2 + ")";
      }
      case "ArrayBuffer": {
        const ui8 = new Uint8Array(thing);
        return `new Uint8Array([${ui8.toString()}]).buffer`;
      }
      case "Temporal.Duration":
      case "Temporal.Instant":
      case "Temporal.PlainDate":
      case "Temporal.PlainTime":
      case "Temporal.PlainDateTime":
      case "Temporal.PlainMonthDay":
      case "Temporal.PlainYearMonth":
      case "Temporal.ZonedDateTime":
        return `${type}.from(${stringify_string(thing.toString())})`;
      default:
        const keys2 = Object.keys(thing);
        const obj = keys2.map((key) => `${safe_key(key)}:${stringify(thing[key])}`).join(",");
        const proto = Object.getPrototypeOf(thing);
        if (proto === null) {
          return keys2.length > 0 ? `{${obj},__proto__:null}` : `{__proto__:null}`;
        }
        return `{${obj}}`;
    }
  }
  const str = stringify(value);
  if (names.size) {
    const params = [];
    const statements = [];
    const values = [];
    const reconstructions = [];
    names.forEach((name, thing) => {
      params.push(name);
      if (custom.has(thing)) {
        values.push(
          /** @type {string} */
          custom.get(thing)
        );
        return;
      }
      if (is_primitive(thing)) {
        values.push(stringify_cached_primitive(thing));
        return;
      }
      const type = get_type(thing);
      switch (type) {
        case "Number":
        case "String":
        case "Boolean":
        case "BigInt": {
          const primitive = thing.valueOf();
          if (names.has(primitive)) {
            values.push("{}");
            reconstructions.push(`${name}=Object(${stringify(primitive)})`);
          } else {
            values.push(`Object(${stringify(primitive)})`);
          }
          break;
        }
        case "RegExp":
          const { source, flags } = thing;
          const regexp = flags ? `new RegExp(${stringify_string(source)},"${flags}")` : `new RegExp(${stringify_string(source)})`;
          values.push(regexp);
          break;
        case "Date":
          values.push(`new Date(${thing.getTime()})`);
          break;
        case "URL":
          values.push(`new URL(${stringify_string(thing.toString())})`);
          break;
        case "URLSearchParams":
          values.push(`new URLSearchParams(${stringify_string(thing.toString())})`);
          break;
        case "Array": {
          const populated_keys = valid_array_indices(thing);
          values.push(
            thing.length > 32 + 2 * populated_keys.length ? stringify_sparse_array(thing.length) : `Array(${thing.length})`
          );
          for (const i of populated_keys) {
            statements.push(`${name}[${i}]=${stringify(thing[i])}`);
          }
          break;
        }
        case "Set": {
          values.push(`new Set`);
          const adds = Array.from(thing).map((v) => `.add(${stringify(v)})`);
          if (adds.length > 0) statements.push(name + adds.join(""));
          break;
        }
        case "Map": {
          values.push(`new Map`);
          const sets = Array.from(thing).map(
            ([k, v]) => `.set(${stringify(k)}, ${stringify(v)})`
          );
          if (sets.length > 0) statements.push(name + sets.join(""));
          break;
        }
        case "Int8Array":
        case "Uint8Array":
        case "Uint8ClampedArray":
        case "Int16Array":
        case "Uint16Array":
        case "Float16Array":
        case "Int32Array":
        case "Uint32Array":
        case "Float32Array":
        case "Float64Array":
        case "BigInt64Array":
        case "BigUint64Array": {
          if (is_buffer(thing)) thing = new Uint8Array(thing);
          let str2 = `new ${type}`;
          if (!names.has(thing.buffer)) {
            str2 += `([${stringify_typed_array_elements(type, thing.buffer)}])`;
          } else {
            str2 += `(${stringify(thing.buffer)})`;
          }
          if (thing.byteLength !== thing.buffer.byteLength) {
            const start = thing.byteOffset / thing.BYTES_PER_ELEMENT;
            const end = start + thing.length;
            str2 += `.subarray(${start},${end})`;
          }
          values.push(`{}`);
          reconstructions.push(`${name}=${str2}`);
          break;
        }
        case "DataView": {
          let str2 = `new DataView`;
          if (!names.has(thing.buffer)) {
            str2 += `(new Uint8Array([${new Uint8Array(thing.buffer)}]).buffer`;
          } else {
            str2 += `(${stringify(thing.buffer)}`;
          }
          if (thing.byteLength !== thing.buffer.byteLength) {
            str2 += `,${thing.byteOffset},${thing.byteLength}`;
          }
          str2 += ")";
          values.push(`{}`);
          reconstructions.push(`${name}=${str2}`);
          break;
        }
        case "ArrayBuffer":
          values.push(`new Uint8Array([${new Uint8Array(thing)}]).buffer`);
          break;
        case "Temporal.Duration":
        case "Temporal.Instant":
        case "Temporal.PlainDate":
        case "Temporal.PlainTime":
        case "Temporal.PlainDateTime":
        case "Temporal.PlainMonthDay":
        case "Temporal.PlainYearMonth":
        case "Temporal.ZonedDateTime":
          values.push(`${type}.from(${stringify_string(thing.toString())})`);
          break;
        default:
          values.push(Object.getPrototypeOf(thing) === null ? "Object.create(null)" : "{}");
          Object.keys(thing).forEach((key) => {
            statements.push(`${name}${safe_prop(key)}=${stringify(thing[key])}`);
          });
      }
    });
    statements.push(`return ${str}`);
    const body = [...reconstructions, ...statements].join(";");
    if (params.length > 65534) {
      return `(function(){var[${params.join(",")}]=arguments[0];${body}}([${values.join(",")}]))`;
    }
    return `(function(${params.join(",")}){${body}}(${values.join(",")}))`;
  } else {
    return str;
  }
}
function stringify_sparse_array(length) {
  return `(function(a){a[${MAX_ARRAY_INDEX}]=0;delete a[${MAX_ARRAY_INDEX}];a.length=${length};return a}([]))`;
}
function stringify_typed_array_elements(type, buffer) {
  const array = new /** @type {any} */
  globalThis[type](buffer);
  if (type === "BigInt64Array" || type === "BigUint64Array") {
    return Array.from(array, (element) => `${element}n`).join(",");
  }
  if (array instanceof Float32Array || array instanceof Float64Array || typeof Float16Array !== "undefined" && array instanceof Float16Array) {
    return Array.from(array, (element) => Object.is(element, -0) ? "-0" : `${element}`).join(",");
  }
  return array.toString();
}
function get_name(num) {
  let name = "";
  do {
    name = chars[num % chars.length] + name;
    num = ~~(num / chars.length) - 1;
  } while (num >= 0);
  return reserved.test(name) ? `${name}0` : name;
}
function escape_unsafe_char(c) {
  return escaped[c] || c;
}
function escape_unsafe_chars(str) {
  return str.replace(unsafe_chars, escape_unsafe_char);
}
function safe_key(key) {
  return /^[_$a-zA-Z][_$a-zA-Z0-9]*$/.test(key) ? key : escape_unsafe_chars(JSON.stringify(key));
}
function safe_prop(key) {
  return /^[_$a-zA-Z][_$a-zA-Z0-9]*$/.test(key) ? `.${key}` : `[${escape_unsafe_chars(JSON.stringify(key))}]`;
}
function stringify_primitive(thing) {
  const type = typeof thing;
  if (type === "string") return stringify_string(thing);
  if (thing === void 0) return "void 0";
  if (thing === 0 && 1 / thing < 0) return "-0";
  const str = String(thing);
  if (type === "number") return str.replace(/^(-)?0\./, "$1.");
  if (type === "bigint") return thing + "n";
  return str;
}
class RenderResult {
  /**
   * @param {() => AccumulatedContent} render
   * @param {() => Promise<AccumulatedContent & { hashes: { script: Sha256Source[] } }>} render_async
   */
  constructor(render2, render_async) {
    __privateAdd(this, _RenderResult_instances);
    /** @type {() => AccumulatedContent} */
    __privateAdd(this, _render);
    /** @type {() => Promise<AccumulatedContent & { hashes: { script: Sha256Source[] } }>} */
    __privateAdd(this, _render_async);
    /** @type {AccumulatedContent | undefined} */
    __privateAdd(this, _sync);
    /** @type {{ script: '' }} */
    __privateAdd(this, _hashes, { script: "" });
    /** @type {Promise<AccumulatedContent & { hashes: { script: Sha256Source[] } }> | undefined} */
    __privateAdd(this, _promise);
    __privateSet(this, _render, render2);
    __privateSet(this, _render_async, render_async);
  }
  get html() {
    return __privateMethod(this, _RenderResult_instances, get_fn).call(this).body;
  }
  get head() {
    return __privateMethod(this, _RenderResult_instances, get_fn).call(this).head;
  }
  get body() {
    return __privateMethod(this, _RenderResult_instances, get_fn).call(this).body;
  }
  get hashes() {
    return __privateGet(this, _hashes);
  }
  /**
   * This is not type-safe, but honestly it's the best I can do right now, and it's a straightforward function.
   *
   * @template TResult1
   * @template [TResult2=never]
   * @param {(value: SyncRenderOutput) => TResult1} onfulfilled
   * @param {(reason: unknown) => TResult2} onrejected
   */
  then(onfulfilled, onrejected) {
    {
      const result = __privateMethod(this, _RenderResult_instances, get_fn).call(this);
      const user_result = onfulfilled({
        head: result.head,
        body: result.body,
        html: result.body,
        hashes: { script: [] }
      });
      return Promise.resolve(user_result);
    }
  }
}
_render = new WeakMap();
_render_async = new WeakMap();
_sync = new WeakMap();
_hashes = new WeakMap();
_promise = new WeakMap();
_RenderResult_instances = new WeakSet();
get_fn = function() {
  return __privateGet(this, _sync) ?? __privateSet(this, _sync, __privateGet(this, _render).call(this));
};
const _Renderer = class _Renderer {
  /**
   * @param {SSRState} global
   * @param {Renderer | undefined} [parent]
   */
  constructor(global, parent) {
    __privateAdd(this, _Renderer_instances);
    /**
     * The contents of the renderer.
     * @type {RendererItem[]}
     */
    __privateAdd(this, _out, []);
    /**
     * Any `onDestroy` callbacks registered during execution of this renderer.
     * @type {(() => void)[] | undefined}
     */
    __privateAdd(this, _on_destroy);
    /**
     * Whether this renderer is a component body.
     * @type {boolean}
     */
    __privateAdd(this, _is_component_body, false);
    /**
     * If set, this renderer is an error boundary. When async collection
     * of the children fails, the failed snippet is rendered instead.
     * @type {{
     * 	failed: (renderer: Renderer, error: unknown, reset: () => void) => void;
     * 	transformError: (error: unknown) => unknown;
     * 	context: SSRContext | null;
     * } | null}
     */
    __privateAdd(this, _boundary, null);
    /**
     * The type of string content that this renderer is accumulating.
     * @type {RendererType}
     */
    __publicField(this, "type");
    /** @type {Renderer | undefined} */
    __privateAdd(this, _parent);
    /**
     * Asynchronous work associated with this renderer
     * @type {Promise<void> | undefined}
     */
    __publicField(this, "promise");
    /**
     * State which is associated with the content tree as a whole.
     * It will be re-exposed, uncopied, on all children.
     * @type {SSRState}
     * @readonly
     */
    __publicField(this, "global");
    /**
     * State that is local to the branch it is declared in.
     * It will be shallow-copied to all children.
     *
     * @type {{ select_value: any, multiple: boolean }}
     */
    __publicField(this, "local");
    __privateSet(this, _parent, parent);
    this.global = global;
    this.local = parent ? { ...parent.local } : { select_value: void 0, multiple: false };
    this.type = parent ? parent.type : "body";
  }
  /**
   * @param {(renderer: Renderer) => void} fn
   */
  head(fn) {
    const head = new _Renderer(this.global, this);
    head.type = "head";
    __privateGet(this, _out).push(head);
    head.child(fn);
  }
  /**
   * @param {Array<Promise<void>>} blockers
   * @param {(renderer: Renderer) => void} fn
   */
  async_block(blockers, fn) {
    __privateGet(this, _out).push(BLOCK_OPEN);
    this.async(blockers, fn);
    __privateGet(this, _out).push(BLOCK_CLOSE);
  }
  /**
   * @param {Array<Promise<void>>} blockers
   * @param {(renderer: Renderer) => void} fn
   */
  async(blockers, fn) {
    let callback = fn;
    if (blockers.length > 0) {
      const context2 = ssr_context;
      callback = (renderer) => {
        return Promise.all(blockers).then(() => {
          const previous_context = ssr_context;
          try {
            set_ssr_context(context2);
            return fn(renderer);
          } finally {
            set_ssr_context(previous_context);
          }
        });
      };
    }
    this.child(callback);
  }
  /**
   * @param {Array<() => void>} thunks
   */
  run(thunks) {
    const context2 = ssr_context;
    let promise = Promise.resolve(thunks[0]());
    const promises = [promise];
    if (context2 !== null && thunks.length > 1) {
      context2.i = true;
    }
    for (const fn of thunks.slice(1)) {
      promise = promise.then(() => {
        const previous_context = ssr_context;
        set_ssr_context(context2);
        try {
          return fn();
        } finally {
          set_ssr_context(previous_context);
        }
      });
      promises.push(promise);
    }
    promise.catch(noop);
    this.promise = this.global.track(promise);
    return promises;
  }
  /**
   * @param {(renderer: Renderer) => MaybePromise<void>} fn
   */
  child_block(fn) {
    __privateGet(this, _out).push(BLOCK_OPEN);
    this.child(fn);
    __privateGet(this, _out).push(BLOCK_CLOSE);
  }
  /**
   * Create a child renderer. The child renderer inherits the state from the parent,
   * but has its own content.
   * @param {(renderer: Renderer) => MaybePromise<void>} fn
   */
  child(fn) {
    const child = new _Renderer(this.global, this);
    __privateGet(this, _out).push(child);
    const parent = ssr_context;
    set_ssr_context({
      ...ssr_context,
      p: parent,
      c: null,
      r: child,
      i: (ssr_context == null ? void 0 : ssr_context.i) ?? false
    });
    const result = fn(child);
    set_ssr_context(parent);
    if (result instanceof Promise) {
      result.catch(noop);
      result.finally(() => set_ssr_context(null)).catch(noop);
      if (child.global.mode === "sync") {
        await_invalid();
      }
      child.promise = child.global.track(result);
    }
    return child;
  }
  /**
   * Render children inside an error boundary. If the children throw and the API-level
   * `transformError` transform handles the error (doesn't re-throw), the `failed` snippet is
   * rendered instead. Otherwise the error propagates.
   *
   * @param {{ failed?: (renderer: Renderer, error: unknown, reset: () => void) => void }} props
   * @param {(renderer: Renderer) => MaybePromise<void>} children_fn
   */
  boundary(props, children_fn) {
    var _a;
    const child = new _Renderer(this.global, this);
    __privateGet(this, _out).push(child);
    const parent_context = ssr_context;
    if (props.failed) {
      __privateSet(child, _boundary, {
        failed: props.failed,
        transformError: this.global.transformError,
        context: parent_context
      });
    }
    set_ssr_context({
      ...ssr_context,
      p: parent_context,
      c: null,
      r: child,
      i: (ssr_context == null ? void 0 : ssr_context.i) ?? false
    });
    try {
      const result = children_fn(child);
      set_ssr_context(parent_context);
      if (result instanceof Promise) {
        if (child.global.mode === "sync") {
          await_invalid();
        }
        result.catch(noop);
        child.promise = child.global.track(result);
      }
    } catch (error) {
      set_ssr_context(parent_context);
      const failed_snippet = props.failed;
      if (!failed_snippet) throw error;
      const result = this.global.transformError(error);
      __privateGet(child, _out).length = 0;
      __privateSet(child, _boundary, null);
      if (result instanceof Promise) {
        if (this.global.mode === "sync") {
          await_invalid();
        }
        child.promise = child.global.track(
          /** @type {Promise<unknown>} */
          result.then((transformed) => {
            var _a2;
            set_ssr_context(parent_context);
            __privateGet(child, _out).push(__privateMethod(_a2 = _Renderer, _Renderer_static, serialize_failed_boundary_fn).call(_a2, transformed));
            failed_snippet(child, transformed, noop);
            __privateGet(child, _out).push(BLOCK_CLOSE);
          })
        );
        child.promise.catch(noop);
      } else {
        __privateGet(child, _out).push(__privateMethod(_a = _Renderer, _Renderer_static, serialize_failed_boundary_fn).call(_a, result));
        failed_snippet(child, result, noop);
        __privateGet(child, _out).push(BLOCK_CLOSE);
      }
    }
  }
  /**
   * Create a component renderer. The component renderer inherits the state from the parent,
   * but has its own content. It is treated as an ordering boundary for ondestroy callbacks.
   * @param {(renderer: Renderer) => MaybePromise<void>} fn
   * @param {Function} [component_fn]
   * @returns {void}
   */
  component(fn, component_fn) {
    push();
    this.child((renderer) => {
      __privateSet(renderer, _is_component_body, true);
      return fn(renderer);
    });
    pop();
  }
  /**
   * @param {Record<string, any>} attrs
   * @param {(renderer: Renderer) => void} fn
   * @param {string | undefined} [css_hash]
   * @param {Record<string, boolean> | undefined} [classes]
   * @param {Record<string, string> | undefined} [styles]
   * @param {number | undefined} [flags]
   * @param {boolean | undefined} [is_rich]
   * @returns {void}
   */
  select(attrs, fn, css_hash, classes, styles, flags, is_rich) {
    const { value, defaultValue, ...select_attrs } = attrs;
    if (select_attrs.multiple === "") select_attrs.multiple = true;
    this.push(`<select${attributes(select_attrs, css_hash, classes, styles, flags)}>`);
    this.child((renderer) => {
      renderer.local.select_value = value === void 0 ? defaultValue : value;
      renderer.local.multiple = !!select_attrs.multiple;
      fn(renderer);
    });
    this.push(`${is_rich ? "<!>" : ""}</select>`);
  }
  /**
   * @param {Record<string, any>} attrs
   * @param {string | number | boolean | ((renderer: Renderer) => void)} body
   * @param {string | undefined} [css_hash]
   * @param {Record<string, boolean> | undefined} [classes]
   * @param {Record<string, string> | undefined} [styles]
   * @param {number | undefined} [flags]
   * @param {boolean | undefined} [is_rich]
   */
  option(attrs, body, css_hash, classes, styles, flags, is_rich) {
    __privateGet(this, _out).push(`<option${attributes(attrs, css_hash, classes, styles, flags)}`);
    const close = (renderer, value, { head, body: body2 }) => {
      if (has_own_property.call(attrs, "value")) {
        value = attrs.value;
      }
      var select_value = this.local.select_value;
      if (
        // Super edge-case, but theoretically someone could use arrays with non-multiple selects,
        // so we gotta check for the multiple attribute presence, too.
        this.local.multiple && is_array(select_value) ? select_value.includes(value) : value === select_value
      ) {
        __privateGet(renderer, _out).push(' selected=""');
      }
      __privateGet(renderer, _out).push(`>${body2}${is_rich ? "<!>" : ""}</option>`);
      if (head) {
        renderer.head((child) => child.push(head));
      }
    };
    if (typeof body === "function") {
      this.child((renderer) => {
        var _a, _b;
        const r2 = new _Renderer(this.global, this);
        body(r2);
        if (this.global.mode === "async") {
          return __privateMethod(_a = r2, _Renderer_instances, collect_content_async_fn).call(_a).then((content) => {
            close(renderer, content.body.replaceAll("<!---->", ""), content);
          });
        } else {
          const content = __privateMethod(_b = r2, _Renderer_instances, collect_content_fn).call(_b);
          close(renderer, content.body.replaceAll("<!---->", ""), content);
        }
      });
    } else {
      close(this, body, { body: escape_html(body) });
    }
  }
  /**
   * @param {(renderer: Renderer) => void} fn
   */
  title(fn) {
    const path = this.get_path();
    const close = (head) => {
      this.global.set_title(head, path);
    };
    this.child((renderer) => {
      var _a, _b;
      const r2 = new _Renderer(renderer.global, renderer);
      fn(r2);
      if (renderer.global.mode === "async") {
        return __privateMethod(_a = r2, _Renderer_instances, collect_content_async_fn).call(_a).then((content) => {
          close(content.head);
        });
      } else {
        const content = __privateMethod(_b = r2, _Renderer_instances, collect_content_fn).call(_b);
        close(content.head);
      }
    });
  }
  /**
   * @param {string | (() => Promise<string>)} content
   */
  push(content) {
    if (typeof content === "function") {
      this.child(async (renderer) => renderer.push(await content()));
    } else {
      __privateGet(this, _out).push(content);
    }
  }
  /**
   * @param {() => void} fn
   */
  on_destroy(fn) {
    (__privateGet(this, _on_destroy) ?? __privateSet(this, _on_destroy, [])).push(fn);
  }
  /**
   * @returns {number[]}
   */
  get_path() {
    return __privateGet(this, _parent) ? [...__privateGet(this, _parent).get_path(), __privateGet(__privateGet(this, _parent), _out).indexOf(this)] : [];
  }
  /**
   * @deprecated this is needed for legacy component bindings
   */
  copy() {
    const copy = new _Renderer(this.global, __privateGet(this, _parent));
    copy.type = this.type;
    __privateSet(copy, _out, __privateGet(this, _out).map((item) => item instanceof _Renderer ? item.copy() : item));
    copy.promise = this.promise;
    return copy;
  }
  /**
   * @param {Renderer} other
   * @deprecated this is needed for legacy component bindings
   */
  subsume(other) {
    if (this.global.mode !== other.global.mode) {
      throw new Error(
        "invariant: A renderer cannot switch modes. If you're seeing this, there's a compiler bug. File an issue!"
      );
    }
    this.local = other.local;
    __privateSet(this, _out, __privateGet(other, _out).map((item, i) => {
      const current = __privateGet(this, _out)[i];
      if (current instanceof _Renderer && item instanceof _Renderer) {
        current.subsume(item);
        return current;
      }
      return item;
    }));
    this.promise = other.promise;
    this.type = other.type;
  }
  get length() {
    return __privateGet(this, _out).length;
  }
  /**
   * Only available on the server and when compiling with the `server` option.
   * Takes a component and returns an object with `body` and `head` properties on it, which you can use to populate the HTML when server-rendering your app.
   * @template {Record<string, any>} Props
   * @param {Component<Props>} component
   * @param {{ props?: Omit<Props, '$$slots' | '$$events'>; context?: Map<any, any>; idPrefix?: string; csp?: Csp }} [options]
   * @returns {RenderOutput}
   */
  static render(component, options = {}) {
    return (
      /** @type {RenderOutput} */
      /** @type {unknown} */
      new RenderResult(
        () => {
          var _a;
          return __privateMethod(_a = _Renderer, _Renderer_static, render_fn).call(_a, component, options);
        },
        () => init_render_context().then(
          () => with_render_context(() => {
            var _a;
            return __privateMethod(_a = _Renderer, _Renderer_static, render_async_fn).call(_a, component, options);
          })
        )
      )
    );
  }
};
_out = new WeakMap();
_on_destroy = new WeakMap();
_is_component_body = new WeakMap();
_boundary = new WeakMap();
_parent = new WeakMap();
_Renderer_static = new WeakSet();
serialize_failed_boundary_fn = function(error) {
  var json = JSON.stringify(error);
  var escaped2 = json.replace(/>/g, "\\u003e").replace(/</g, "\\u003c");
  return `<!--${HYDRATION_START_FAILED}${escaped2}-->`;
};
_Renderer_instances = new WeakSet();
collect_on_destroy_fn = function* () {
  var _a;
  for (const component of __privateMethod(this, _Renderer_instances, traverse_components_fn).call(this)) {
    yield* __privateMethod(_a = component, _Renderer_instances, collect_ondestroy_fn).call(_a);
  }
};
traverse_components_fn = function* () {
  var _a;
  for (const child of __privateGet(this, _out)) {
    if (typeof child !== "string") {
      yield* __privateMethod(_a = child, _Renderer_instances, traverse_components_fn).call(_a);
    }
  }
  if (__privateGet(this, _is_component_body)) {
    yield this;
  }
};
collect_ondestroy_fn = function* () {
  var _a;
  if (__privateGet(this, _on_destroy)) {
    for (const fn of __privateGet(this, _on_destroy)) {
      yield fn;
    }
  }
  for (const child of __privateGet(this, _out)) {
    if (child instanceof _Renderer && !__privateGet(child, _is_component_body)) {
      yield* __privateMethod(_a = child, _Renderer_instances, collect_ondestroy_fn).call(_a);
    }
  }
};
/**
 * Runs every `onDestroy` callback in this renderer tree. On a failed render,
 * cleanup errors are suppressed so they do not mask the render error.
 * @param {boolean} suppress_errors
 */
run_on_destroy_fn = function(suppress_errors) {
  let first_error;
  let has_error = false;
  for (const cleanup of __privateMethod(this, _Renderer_instances, collect_on_destroy_fn).call(this)) {
    try {
      cleanup();
    } catch (error) {
      if (!suppress_errors && !has_error) {
        first_error = error;
        has_error = true;
      }
    }
  }
  if (has_error) throw first_error;
};
create_fn = function(mode, options) {
  var _a;
  if ((_a = options.idPrefix) == null ? void 0 : _a.includes("--")) {
    invalid_id_prefix();
  }
  return new _Renderer(
    new SSRState(
      mode,
      options.idPrefix ? options.idPrefix + "-" : "",
      options.csp,
      options.transformError
    )
  );
};
render_fn = function(component, options) {
  var _a, _b, _c, _d, _e;
  var previous_context = ssr_context;
  const renderer = __privateMethod(_a = _Renderer, _Renderer_static, create_fn).call(_a, "sync", options);
  let result;
  let render_error;
  let failed = false;
  try {
    try {
      __privateMethod(_b = _Renderer, _Renderer_static, open_render_fn).call(_b, renderer, component, options);
      result = __privateMethod(_d = _Renderer, _Renderer_static, close_render_fn).call(_d, __privateMethod(_c = renderer, _Renderer_instances, collect_content_fn).call(_c), renderer);
    } catch (error) {
      render_error = error;
      failed = true;
    }
    __privateMethod(_e = renderer, _Renderer_instances, run_on_destroy_fn).call(_e, failed);
    if (failed) throw render_error;
    return (
      /** @type {AccumulatedContent} */
      result
    );
  } finally {
    renderer.global.abort();
    set_ssr_context(previous_context);
  }
};
render_async_fn = async function(component, options) {
  var _a, _b, _c, _d, _e, _f;
  const previous_context = ssr_context;
  const renderer = __privateMethod(_a = _Renderer, _Renderer_static, create_fn).call(_a, "async", options);
  let result;
  let render_error;
  let failed = false;
  try {
    try {
      __privateMethod(_b = _Renderer, _Renderer_static, open_render_fn).call(_b, renderer, component, options);
      const content = await __privateMethod(_c = renderer, _Renderer_instances, collect_content_async_fn).call(_c);
      const hydratables = await __privateMethod(_d = renderer, _Renderer_instances, collect_hydratables_fn).call(_d);
      if (hydratables !== null) {
        content.head = hydratables + content.head;
      }
      result = __privateMethod(_e = _Renderer, _Renderer_static, close_render_fn).call(_e, content, renderer);
    } catch (error) {
      render_error = error;
      failed = true;
      renderer.global.abort();
      await renderer.global.settle();
    }
    __privateMethod(_f = renderer, _Renderer_instances, run_on_destroy_fn).call(_f, failed);
    if (failed) throw render_error;
    return (
      /** @type {AccumulatedContent & { hashes: { script: Sha256Source[] } }} */
      result
    );
  } finally {
    set_ssr_context(previous_context);
    renderer.global.abort();
  }
};
/**
 * Collect all of the code from the `out` array and return it as a string, or a promise resolving to a string.
 * @param {AccumulatedContent} content
 * @returns {AccumulatedContent}
 */
collect_content_fn = function(content = { head: "", body: "" }) {
  var _a;
  for (const item of __privateGet(this, _out)) {
    if (typeof item === "string") {
      content[this.type] += item;
    } else if (item instanceof _Renderer) {
      __privateMethod(_a = item, _Renderer_instances, collect_content_fn).call(_a, content);
    }
  }
  return content;
};
collect_content_async_fn = async function(content = { head: "", body: "" }) {
  var _a, _b, _c, _d;
  await this.promise;
  for (const item of __privateGet(this, _out)) {
    if (typeof item === "string") {
      content[this.type] += item;
    } else if (item instanceof _Renderer) {
      if (__privateGet(item, _boundary)) {
        const boundary_content = { head: "", body: "" };
        try {
          await __privateMethod(_a = item, _Renderer_instances, collect_content_async_fn).call(_a, boundary_content);
          content.head += boundary_content.head;
          content.body += boundary_content.body;
        } catch (error) {
          const { context: context2, failed, transformError } = __privateGet(item, _boundary);
          set_ssr_context(context2);
          let promise = transformError(error);
          set_ssr_context(null);
          let transformed = await promise;
          set_ssr_context(context2);
          const failed_renderer = new _Renderer(item.global, item);
          failed_renderer.type = item.type;
          __privateGet(failed_renderer, _out).push(__privateMethod(_b = _Renderer, _Renderer_static, serialize_failed_boundary_fn).call(_b, transformed));
          failed(failed_renderer, transformed, noop);
          __privateGet(failed_renderer, _out).push(BLOCK_CLOSE);
          await __privateMethod(_c = failed_renderer, _Renderer_instances, collect_content_async_fn).call(_c, content);
        }
      } else {
        await __privateMethod(_d = item, _Renderer_instances, collect_content_async_fn).call(_d, content);
      }
    }
  }
  return content;
};
collect_hydratables_fn = async function() {
  var _a;
  const ctx = get_render_context().hydratable;
  for (const [_, key] of ctx.unresolved_promises) {
    unresolved_hydratable(key, ((_a = ctx.lookup.get(key)) == null ? void 0 : _a.stack) ?? "<missing stack trace>");
  }
  for (const comparison of ctx.comparisons) {
    await comparison;
  }
  return await __privateMethod(this, _Renderer_instances, hydratable_block_fn).call(this, ctx);
};
open_render_fn = function(renderer, component, options) {
  var previous_context = ssr_context;
  try {
    const context2 = { p: null, c: options.context ?? null, r: renderer, i: false };
    set_ssr_context(context2);
    renderer.push(BLOCK_OPEN);
    component(renderer, options.props ?? {});
    renderer.push(BLOCK_CLOSE);
  } finally {
    set_ssr_context(previous_context);
  }
};
close_render_fn = function(content, renderer) {
  let head = content.head + renderer.global.get_title();
  let body = content.body;
  for (const { hash, code } of renderer.global.css) {
    head += `<style id="${hash}">${code}</style>`;
  }
  return {
    head,
    body,
    hashes: {
      script: renderer.global.csp.script_hashes
    }
  };
};
hydratable_block_fn = async function(ctx) {
  if (ctx.lookup.size === 0) {
    return null;
  }
  let entries = [];
  let has_promises = false;
  for (const [k, v] of ctx.lookup) {
    if (v.promises) {
      has_promises = true;
      for (const p of v.promises) await p;
    }
    entries.push(`[${uneval(k)},${v.serialized}]`);
  }
  let prelude = `const h = (window.__svelte ??= {}).h ??= new Map();`;
  if (has_promises) {
    prelude = `const r = (v) => Promise.resolve(v);
				${prelude}`;
  }
  const body = `
			{
				${prelude}

				for (const [k, v] of [
					${entries.join(",\n					")}
				]) {
					h.set(k, v);
				}
			}
		`;
  let csp_attr = "";
  if (this.global.csp.nonce) {
    csp_attr = ` nonce="${this.global.csp.nonce}"`;
  } else if (this.global.csp.hash) {
    const hash = await sha256(body);
    this.global.csp.script_hashes.push(`sha256-${hash}`);
  }
  return `
		<script${csp_attr}>${body}<\/script>`;
};
__privateAdd(_Renderer, _Renderer_static);
let Renderer = _Renderer;
class SSRState {
  /**
   * @param {'sync' | 'async'} mode
   * @param {string} id_prefix
   * @param {Csp} csp
   * @param {((error: unknown) => unknown) | undefined} [transformError]
   */
  constructor(mode, id_prefix = "", csp = { hash: false }, transformError) {
    /** @readonly @type {Csp & { script_hashes: Sha256Source[] }} */
    __publicField(this, "csp");
    /** @readonly @type {'sync' | 'async'} */
    __publicField(this, "mode");
    /** @readonly @type {() => string} */
    __publicField(this, "uid");
    /** @readonly @type {Set<{ hash: string; code: string }>} */
    __publicField(this, "css", /* @__PURE__ */ new Set());
    /** @type {Set<Promise<unknown>>} */
    __privateAdd(this, _pending, /* @__PURE__ */ new Set());
    /** @type {AbortController | null} */
    __privateAdd(this, _controller, null);
    __privateAdd(this, _aborted, false);
    /**
     * `transformError` passed to `render`. Called when an error boundary catches an error.
     * Throws by default if unset in `render`.
     * @type {(error: unknown) => unknown}
     */
    __publicField(this, "transformError");
    /** @type {{ path: number[], value: string }} */
    __privateAdd(this, _title, { path: [], value: "" });
    this.mode = mode;
    this.csp = { ...csp, script_hashes: [] };
    this.transformError = transformError ?? ((error) => {
      throw error;
    });
    let uid = 1;
    this.uid = () => `${id_prefix}s${uid++}`;
  }
  /**
   * @template T
   * @param {Promise<T>} promise
   * @returns {Promise<T>}
   */
  track(promise) {
    __privateGet(this, _pending).add(promise);
    promise.then(
      () => __privateGet(this, _pending).delete(promise),
      () => __privateGet(this, _pending).delete(promise)
    );
    return promise;
  }
  async settle() {
    while (__privateGet(this, _pending).size > 0) {
      await Promise.allSettled([...__privateGet(this, _pending)]);
    }
  }
  abort() {
    var _a;
    if (__privateGet(this, _aborted)) return;
    __privateSet(this, _aborted, true);
    (_a = __privateGet(this, _controller)) == null ? void 0 : _a.abort(STALE_REACTION);
  }
  get_abort_signal() {
    const controller = __privateGet(this, _controller) ?? __privateSet(this, _controller, new AbortController());
    if (__privateGet(this, _aborted)) controller.abort(STALE_REACTION);
    return controller.signal;
  }
  get_title() {
    return __privateGet(this, _title).value;
  }
  /**
   * Performs a depth-first (lexicographic) comparison using the path. Rejects sets
   * from earlier than or equal to the current value.
   * @param {string} value
   * @param {number[]} path
   */
  set_title(value, path) {
    const current = __privateGet(this, _title).path;
    let i = 0;
    let l = Math.min(path.length, current.length);
    while (i < l && path[i] === current[i]) i += 1;
    if (path[i] === void 0) return;
    if (current[i] === void 0 || path[i] > current[i]) {
      __privateGet(this, _title).path = path;
      __privateGet(this, _title).value = value;
    }
  }
}
_pending = new WeakMap();
_controller = new WeakMap();
_aborted = new WeakMap();
_title = new WeakMap();
const INVALID_ATTR_NAME_CHAR_REGEX = /[\s'">/=\u{FDD0}-\u{FDEF}\u{FFFE}\u{FFFF}\u{1FFFE}\u{1FFFF}\u{2FFFE}\u{2FFFF}\u{3FFFE}\u{3FFFF}\u{4FFFE}\u{4FFFF}\u{5FFFE}\u{5FFFF}\u{6FFFE}\u{6FFFF}\u{7FFFE}\u{7FFFF}\u{8FFFE}\u{8FFFF}\u{9FFFE}\u{9FFFF}\u{AFFFE}\u{AFFFF}\u{BFFFE}\u{BFFFF}\u{CFFFE}\u{CFFFF}\u{DFFFE}\u{DFFFF}\u{EFFFE}\u{EFFFF}\u{FFFFE}\u{FFFFF}\u{10FFFE}\u{10FFFF}]/u;
function render(component, options = {}) {
  var _a;
  if (((_a = options.csp) == null ? void 0 : _a.hash) && options.csp.nonce) {
    invalid_csp();
  }
  return Renderer.render(
    /** @type {Component<Props>} */
    component,
    options
  );
}
function attributes(attrs, css_hash, classes, styles, flags = 0) {
  if (styles) {
    attrs.style = to_style(attrs.style, styles);
  }
  if (attrs.class) {
    attrs.class = clsx(attrs.class);
  }
  if (css_hash || classes) {
    attrs.class = to_class(attrs.class, css_hash, classes);
  }
  let attr_str = "";
  let name;
  const is_html = (flags & ELEMENT_IS_NAMESPACED) === 0;
  const lowercase = (flags & ELEMENT_PRESERVE_ATTRIBUTE_CASE) === 0;
  const is_input = (flags & ELEMENT_IS_INPUT) !== 0;
  const names = Object.keys(attrs);
  outer: for (let i = 0; i < names.length; i++) {
    name = names[i];
    if (typeof attrs[name] === "function") continue;
    if (name[0] === "$" && name[1] === "$") continue;
    if (name === "" || INVALID_ATTR_NAME_CHAR_REGEX.test(name)) continue;
    var value = attrs[name];
    var lower = name.toLowerCase();
    if (lowercase) name = lower;
    if (lower.length > 2 && lower.startsWith("on")) continue;
    if (is_input) {
      if (name === "defaultvalue" || name === "defaultchecked") {
        name = name === "defaultvalue" ? "value" : "checked";
        if (name in attrs) continue;
        for (let j = 0; j < names.length; j++) {
          if (names[j].toLowerCase() === name) continue outer;
        }
      }
    }
    attr_str += attr(name, value, is_html && is_boolean_attribute(name));
  }
  return attr_str;
}
function ensure_array_like(array_like_or_iterator) {
  if (array_like_or_iterator) {
    return array_like_or_iterator.length !== void 0 ? array_like_or_iterator : Array.from(array_like_or_iterator);
  }
  return [];
}
function once(get_value) {
  let value = (
    /** @type {V} */
    UNINITIALIZED
  );
  return () => {
    if (value === UNINITIALIZED) {
      value = get_value();
    }
    return value;
  };
}
function derived(fn) {
  const get_value = ssr_context === null ? fn : once(fn);
  let updated_value;
  return function(new_value) {
    if (arguments.length === 0) {
      return updated_value ?? get_value();
    }
    updated_value = new_value;
    return updated_value;
  };
}
const concatArrays$1 = (array1, array2) => {
  const combinedArray = new Array(array1.length + array2.length);
  for (let i = 0; i < array1.length; i++) {
    combinedArray[i] = array1[i];
  }
  for (let i = 0; i < array2.length; i++) {
    combinedArray[array1.length + i] = array2[i];
  }
  return combinedArray;
};
const createClassValidatorObject$1 = (classGroupId, validator) => ({
  classGroupId,
  validator
});
const createClassPartObject$1 = (nextPart = /* @__PURE__ */ new Map(), validators = null, classGroupId) => ({
  nextPart,
  validators,
  classGroupId
});
const CLASS_PART_SEPARATOR$1 = "-";
const EMPTY_CONFLICTS$1 = [];
const ARBITRARY_PROPERTY_PREFIX$1 = "arbitrary..";
const createClassGroupUtils$1 = (config) => {
  const classMap = createClassMap$1(config);
  const {
    conflictingClassGroups,
    conflictingClassGroupModifiers
  } = config;
  const getClassGroupId = (className) => {
    if (className.startsWith("[") && className.endsWith("]")) {
      return getGroupIdForArbitraryProperty$1(className);
    }
    const classParts = className.split(CLASS_PART_SEPARATOR$1);
    const startIndex = classParts[0] === "" && classParts.length > 1 ? 1 : 0;
    return getGroupRecursive$1(classParts, startIndex, classMap);
  };
  const getConflictingClassGroupIds = (classGroupId, hasPostfixModifier) => {
    if (hasPostfixModifier) {
      const modifierConflicts = conflictingClassGroupModifiers[classGroupId];
      const baseConflicts = conflictingClassGroups[classGroupId];
      if (modifierConflicts) {
        if (baseConflicts) {
          return concatArrays$1(baseConflicts, modifierConflicts);
        }
        return modifierConflicts;
      }
      return baseConflicts || EMPTY_CONFLICTS$1;
    }
    return conflictingClassGroups[classGroupId] || EMPTY_CONFLICTS$1;
  };
  return {
    getClassGroupId,
    getConflictingClassGroupIds
  };
};
const getGroupRecursive$1 = (classParts, startIndex, classPartObject) => {
  const classPathsLength = classParts.length - startIndex;
  if (classPathsLength === 0) {
    return classPartObject.classGroupId;
  }
  const currentClassPart = classParts[startIndex];
  const nextClassPartObject = classPartObject.nextPart.get(currentClassPart);
  if (nextClassPartObject) {
    const result = getGroupRecursive$1(classParts, startIndex + 1, nextClassPartObject);
    if (result) return result;
  }
  const validators = classPartObject.validators;
  if (validators === null) {
    return void 0;
  }
  const classRest = startIndex === 0 ? classParts.join(CLASS_PART_SEPARATOR$1) : classParts.slice(startIndex).join(CLASS_PART_SEPARATOR$1);
  const validatorsLength = validators.length;
  for (let i = 0; i < validatorsLength; i++) {
    const validatorObj = validators[i];
    if (validatorObj.validator(classRest)) {
      return validatorObj.classGroupId;
    }
  }
  return void 0;
};
const getGroupIdForArbitraryProperty$1 = (className) => className.slice(1, -1).indexOf(":") === -1 ? void 0 : (() => {
  const content = className.slice(1, -1);
  const colonIndex = content.indexOf(":");
  const property = content.slice(0, colonIndex);
  return property ? ARBITRARY_PROPERTY_PREFIX$1 + property : void 0;
})();
const createClassMap$1 = (config) => {
  const {
    theme,
    classGroups
  } = config;
  return processClassGroups$1(classGroups, theme);
};
const processClassGroups$1 = (classGroups, theme) => {
  const classMap = createClassPartObject$1();
  for (const classGroupId in classGroups) {
    const group = classGroups[classGroupId];
    processClassesRecursively$1(group, classMap, classGroupId, theme);
  }
  return classMap;
};
const processClassesRecursively$1 = (classGroup, classPartObject, classGroupId, theme) => {
  const len = classGroup.length;
  for (let i = 0; i < len; i++) {
    const classDefinition = classGroup[i];
    processClassDefinition$1(classDefinition, classPartObject, classGroupId, theme);
  }
};
const processClassDefinition$1 = (classDefinition, classPartObject, classGroupId, theme) => {
  if (typeof classDefinition === "string") {
    processStringDefinition$1(classDefinition, classPartObject, classGroupId);
    return;
  }
  if (typeof classDefinition === "function") {
    processFunctionDefinition$1(classDefinition, classPartObject, classGroupId, theme);
    return;
  }
  processObjectDefinition$1(classDefinition, classPartObject, classGroupId, theme);
};
const processStringDefinition$1 = (classDefinition, classPartObject, classGroupId) => {
  const classPartObjectToEdit = classDefinition === "" ? classPartObject : getPart$1(classPartObject, classDefinition);
  classPartObjectToEdit.classGroupId = classGroupId;
};
const processFunctionDefinition$1 = (classDefinition, classPartObject, classGroupId, theme) => {
  if (isThemeGetter$1(classDefinition)) {
    processClassesRecursively$1(classDefinition(theme), classPartObject, classGroupId, theme);
    return;
  }
  if (classPartObject.validators === null) {
    classPartObject.validators = [];
  }
  classPartObject.validators.push(createClassValidatorObject$1(classGroupId, classDefinition));
};
const processObjectDefinition$1 = (classDefinition, classPartObject, classGroupId, theme) => {
  const entries = Object.entries(classDefinition);
  const len = entries.length;
  for (let i = 0; i < len; i++) {
    const [key, value] = entries[i];
    processClassesRecursively$1(value, getPart$1(classPartObject, key), classGroupId, theme);
  }
};
const getPart$1 = (classPartObject, path) => {
  let current = classPartObject;
  const parts = path.split(CLASS_PART_SEPARATOR$1);
  const len = parts.length;
  for (let i = 0; i < len; i++) {
    const part = parts[i];
    let next = current.nextPart.get(part);
    if (!next) {
      next = createClassPartObject$1();
      current.nextPart.set(part, next);
    }
    current = next;
  }
  return current;
};
const isThemeGetter$1 = (func) => "isThemeGetter" in func && func.isThemeGetter === true;
const createLruCache = (maxCacheSize) => {
  if (maxCacheSize < 1) {
    return {
      get: () => void 0,
      set: () => {
      }
    };
  }
  let cacheSize = 0;
  let cache = /* @__PURE__ */ Object.create(null);
  let previousCache = /* @__PURE__ */ Object.create(null);
  const update = (key, value) => {
    cache[key] = value;
    cacheSize++;
    if (cacheSize > maxCacheSize) {
      cacheSize = 0;
      previousCache = cache;
      cache = /* @__PURE__ */ Object.create(null);
    }
  };
  return {
    get(key) {
      let value = cache[key];
      if (value !== void 0) {
        return value;
      }
      if ((value = previousCache[key]) !== void 0) {
        update(key, value);
        return value;
      }
    },
    set(key, value) {
      if (key in cache) {
        cache[key] = value;
      } else {
        update(key, value);
      }
    }
  };
};
const IMPORTANT_MODIFIER$1 = "!";
const MODIFIER_SEPARATOR = ":";
const EMPTY_MODIFIERS = [];
const createResultObject$1 = (modifiers, hasImportantModifier, baseClassName, maybePostfixModifierPosition, isExternal) => ({
  modifiers,
  hasImportantModifier,
  baseClassName,
  maybePostfixModifierPosition,
  isExternal
});
const createParseClassName = (config) => {
  const {
    prefix,
    experimentalParseClassName
  } = config;
  let parseClassName2 = (className) => {
    const modifiers = [];
    let bracketDepth = 0;
    let parenDepth = 0;
    let modifierStart = 0;
    let postfixModifierPosition;
    const len = className.length;
    for (let index = 0; index < len; index++) {
      const currentCharacter = className[index];
      if (bracketDepth === 0 && parenDepth === 0) {
        if (currentCharacter === MODIFIER_SEPARATOR) {
          modifiers.push(className.slice(modifierStart, index));
          modifierStart = index + 1;
          continue;
        }
        if (currentCharacter === "/") {
          postfixModifierPosition = index;
          continue;
        }
      }
      if (currentCharacter === "[") bracketDepth++;
      else if (currentCharacter === "]") bracketDepth--;
      else if (currentCharacter === "(") parenDepth++;
      else if (currentCharacter === ")") parenDepth--;
    }
    const baseClassNameWithImportantModifier = modifiers.length === 0 ? className : className.slice(modifierStart);
    let baseClassName = baseClassNameWithImportantModifier;
    let hasImportantModifier = false;
    if (baseClassNameWithImportantModifier.endsWith(IMPORTANT_MODIFIER$1)) {
      baseClassName = baseClassNameWithImportantModifier.slice(0, -1);
      hasImportantModifier = true;
    } else if (
      /**
       * In Tailwind CSS v3 the important modifier was at the start of the base class name. This is still supported for legacy reasons.
       * @see https://github.com/dcastil/tailwind-merge/issues/513#issuecomment-2614029864
       */
      baseClassNameWithImportantModifier.startsWith(IMPORTANT_MODIFIER$1)
    ) {
      baseClassName = baseClassNameWithImportantModifier.slice(1);
      hasImportantModifier = true;
    }
    const maybePostfixModifierPosition = postfixModifierPosition && postfixModifierPosition > modifierStart ? postfixModifierPosition - modifierStart : void 0;
    return createResultObject$1(modifiers, hasImportantModifier, baseClassName, maybePostfixModifierPosition);
  };
  if (prefix) {
    const fullPrefix = prefix + MODIFIER_SEPARATOR;
    const parseClassNameOriginal = parseClassName2;
    parseClassName2 = (className) => className.startsWith(fullPrefix) ? parseClassNameOriginal(className.slice(fullPrefix.length)) : createResultObject$1(EMPTY_MODIFIERS, false, className, void 0, true);
  }
  if (experimentalParseClassName) {
    const parseClassNameOriginal = parseClassName2;
    parseClassName2 = (className) => experimentalParseClassName({
      className,
      parseClassName: parseClassNameOriginal
    });
  }
  return parseClassName2;
};
const createSortModifiers$1 = (config) => {
  const modifierWeights = /* @__PURE__ */ new Map();
  config.orderSensitiveModifiers.forEach((mod, index) => {
    modifierWeights.set(mod, 1e6 + index);
  });
  return (modifiers) => {
    const result = [];
    let currentSegment = [];
    for (let i = 0; i < modifiers.length; i++) {
      const modifier = modifiers[i];
      const isArbitrary = modifier[0] === "[";
      const isOrderSensitive = modifierWeights.has(modifier);
      if (isArbitrary || isOrderSensitive) {
        if (currentSegment.length > 0) {
          currentSegment.sort();
          result.push(...currentSegment);
          currentSegment = [];
        }
        result.push(modifier);
      } else {
        currentSegment.push(modifier);
      }
    }
    if (currentSegment.length > 0) {
      currentSegment.sort();
      result.push(...currentSegment);
    }
    return result;
  };
};
const createConfigUtils$1 = (config) => ({
  cache: createLruCache(config.cacheSize),
  parseClassName: createParseClassName(config),
  sortModifiers: createSortModifiers$1(config),
  postfixLookupClassGroupIds: createPostfixLookupClassGroupIds$1(config),
  ...createClassGroupUtils$1(config)
});
const createPostfixLookupClassGroupIds$1 = (config) => {
  const lookup = /* @__PURE__ */ Object.create(null);
  const classGroupIds = config.postfixLookupClassGroups;
  if (classGroupIds) {
    for (let i = 0; i < classGroupIds.length; i++) {
      lookup[classGroupIds[i]] = true;
    }
  }
  return lookup;
};
const SPLIT_CLASSES_REGEX = /\s+/;
const mergeClassList = (classList, configUtils) => {
  const {
    parseClassName: parseClassName2,
    getClassGroupId,
    getConflictingClassGroupIds,
    sortModifiers,
    postfixLookupClassGroupIds
  } = configUtils;
  const classGroupsInConflict = [];
  const classNames = classList.trim().split(SPLIT_CLASSES_REGEX);
  let result = "";
  for (let index = classNames.length - 1; index >= 0; index -= 1) {
    const originalClassName = classNames[index];
    const {
      isExternal,
      modifiers,
      hasImportantModifier,
      baseClassName,
      maybePostfixModifierPosition
    } = parseClassName2(originalClassName);
    if (isExternal) {
      result = originalClassName + (result.length > 0 ? " " + result : result);
      continue;
    }
    let hasPostfixModifier = !!maybePostfixModifierPosition;
    let classGroupId;
    if (hasPostfixModifier) {
      const baseClassNameWithoutPostfix = baseClassName.substring(0, maybePostfixModifierPosition);
      classGroupId = getClassGroupId(baseClassNameWithoutPostfix);
      const classGroupIdWithPostfix = classGroupId && postfixLookupClassGroupIds[classGroupId] ? getClassGroupId(baseClassName) : void 0;
      if (classGroupIdWithPostfix && classGroupIdWithPostfix !== classGroupId) {
        classGroupId = classGroupIdWithPostfix;
        hasPostfixModifier = false;
      }
    } else {
      classGroupId = getClassGroupId(baseClassName);
    }
    if (!classGroupId) {
      if (!hasPostfixModifier) {
        result = originalClassName + (result.length > 0 ? " " + result : result);
        continue;
      }
      classGroupId = getClassGroupId(baseClassName);
      if (!classGroupId) {
        result = originalClassName + (result.length > 0 ? " " + result : result);
        continue;
      }
      hasPostfixModifier = false;
    }
    const variantModifier = modifiers.length === 0 ? "" : modifiers.length === 1 ? modifiers[0] : sortModifiers(modifiers).join(":");
    const modifierId = hasImportantModifier ? variantModifier + IMPORTANT_MODIFIER$1 : variantModifier;
    const classId = modifierId + classGroupId;
    if (classGroupsInConflict.indexOf(classId) > -1) {
      continue;
    }
    classGroupsInConflict.push(classId);
    const conflictGroups = getConflictingClassGroupIds(classGroupId, hasPostfixModifier);
    for (let i = 0; i < conflictGroups.length; ++i) {
      const group = conflictGroups[i];
      classGroupsInConflict.push(modifierId + group);
    }
    result = originalClassName + (result.length > 0 ? " " + result : result);
  }
  return result;
};
const twJoin = (...classLists) => {
  let index = 0;
  let argument;
  let resolvedValue;
  let string = "";
  while (index < classLists.length) {
    if (argument = classLists[index++]) {
      if (resolvedValue = toValue(argument)) {
        string && (string += " ");
        string += resolvedValue;
      }
    }
  }
  return string;
};
const toValue = (mix) => {
  if (typeof mix === "string") {
    return mix;
  }
  let resolvedValue;
  let string = "";
  for (let k = 0; k < mix.length; k++) {
    if (mix[k]) {
      if (resolvedValue = toValue(mix[k])) {
        string && (string += " ");
        string += resolvedValue;
      }
    }
  }
  return string;
};
const createTailwindMerge$1 = (createConfigFirst, ...createConfigRest) => {
  let configUtils;
  let cacheGet;
  let cacheSet;
  let functionToCall;
  const initTailwindMerge = (classList) => {
    const config = createConfigRest.reduce((previousConfig, createConfigCurrent) => createConfigCurrent(previousConfig), createConfigFirst());
    configUtils = createConfigUtils$1(config);
    cacheGet = configUtils.cache.get;
    cacheSet = configUtils.cache.set;
    functionToCall = tailwindMerge;
    return tailwindMerge(classList);
  };
  const tailwindMerge = (classList) => {
    const cachedResult = cacheGet(classList);
    if (cachedResult) {
      return cachedResult;
    }
    const result = mergeClassList(classList, configUtils);
    cacheSet(classList, result);
    return result;
  };
  functionToCall = initTailwindMerge;
  return (...args) => functionToCall(twJoin(...args));
};
const fallbackThemeArr$1 = [];
const fromTheme$1 = (key) => {
  const themeGetter = (theme) => theme[key] || fallbackThemeArr$1;
  themeGetter.isThemeGetter = true;
  themeGetter.themeKey = key;
  return themeGetter;
};
const arbitraryValueRegex$1 = /^\[(?:(\w[\w-]*):)?(.+)\]$/i;
const arbitraryVariableRegex$1 = /^\((?:(\w[\w-]*):)?(.+)\)$/i;
const fractionRegex$1 = /^\d+(?:\.\d+)?\/\d+(?:\.\d+)?$/;
const tshirtUnitRegex$1 = /^(\d+(\.\d+)?)?(xs|sm|md|lg|xl)$/;
const lengthUnitRegex$1 = /\d+(%|px|r?em|[sdl]?v([hwib]|min|max)|pt|pc|in|cm|mm|cap|ch|ex|r?lh|cq(w|h|i|b|min|max))|\b(calc|min|max|clamp)\(.+\)|^0$/;
const colorFunctionRegex$1 = /^(rgba?|hsla?|hwb|(ok)?(lab|lch)|color-mix|color|light-dark)\(.+\)$/;
const shadowRegex$1 = /^(inset_)?-?((\d+)?\.?(\d+)[a-z]+|0)_-?((\d+)?\.?(\d+)[a-z]+|0)/;
const imageRegex$1 = /^(url|image|image-set|cross-fade|element|(repeating-)?(linear|radial|conic)-gradient)\(.+\)$/;
const isFraction$1 = (value) => fractionRegex$1.test(value);
const isNumber$1 = (value) => !!value && !Number.isNaN(Number(value));
const isInteger$1 = (value) => !!value && Number.isInteger(Number(value));
const isPercent$1 = (value) => value.endsWith("%") && isNumber$1(value.slice(0, -1));
const isTshirtSize$1 = (value) => tshirtUnitRegex$1.test(value);
const isAny$1 = () => true;
const isLengthOnly$1 = (value) => (
  // `colorFunctionRegex` check is necessary because color functions can have percentages in them which which would be incorrectly classified as lengths.
  // For example, `hsl(0 0% 0%)` would be classified as a length without this check.
  // I could also use lookbehind assertion in `lengthUnitRegex` but that isn't supported widely enough.
  lengthUnitRegex$1.test(value) && !colorFunctionRegex$1.test(value)
);
const isNever$1 = () => false;
const isShadow$1 = (value) => shadowRegex$1.test(value);
const isImage$1 = (value) => imageRegex$1.test(value);
const isAnyNonArbitrary$1 = (value) => !isArbitraryValue$1(value) && !isArbitraryVariable$1(value);
const isNamedContainerQuery$1 = (value) => value.startsWith("@container") && (value[10] === "/" && value[11] !== void 0 || value[11] === "s" && value[16] !== void 0 && value.startsWith("-size/", 10) || value[11] === "n" && value[18] !== void 0 && value.startsWith("-normal/", 10));
const isArbitrarySize$1 = (value) => getIsArbitraryValue$1(value, isLabelSize$1, isNever$1);
const isArbitraryValue$1 = (value) => arbitraryValueRegex$1.test(value);
const isArbitraryLength$1 = (value) => getIsArbitraryValue$1(value, isLabelLength$1, isLengthOnly$1);
const isArbitraryNumber$1 = (value) => getIsArbitraryValue$1(value, isLabelNumber$1, isNumber$1);
const isArbitraryWeight$1 = (value) => getIsArbitraryValue$1(value, isLabelWeight$1, isAny$1);
const isArbitraryFamilyName$1 = (value) => getIsArbitraryValue$1(value, isLabelFamilyName$1, isNever$1);
const isArbitraryPosition$1 = (value) => getIsArbitraryValue$1(value, isLabelPosition$1, isNever$1);
const isArbitraryImage$1 = (value) => getIsArbitraryValue$1(value, isLabelImage$1, isImage$1);
const isArbitraryShadow$1 = (value) => getIsArbitraryValue$1(value, isLabelShadow$1, isShadow$1);
const isArbitraryVariable$1 = (value) => arbitraryVariableRegex$1.test(value);
const isArbitraryVariableLength$1 = (value) => getIsArbitraryVariable$1(value, isLabelLength$1);
const isArbitraryVariableFamilyName$1 = (value) => getIsArbitraryVariable$1(value, isLabelFamilyName$1);
const isArbitraryVariablePosition$1 = (value) => getIsArbitraryVariable$1(value, isLabelPosition$1);
const isArbitraryVariableSize$1 = (value) => getIsArbitraryVariable$1(value, isLabelSize$1);
const isArbitraryVariableImage$1 = (value) => getIsArbitraryVariable$1(value, isLabelImage$1);
const isArbitraryVariableShadow$1 = (value) => getIsArbitraryVariable$1(value, isLabelShadow$1, true);
const isArbitraryVariableWeight$1 = (value) => getIsArbitraryVariable$1(value, isLabelWeight$1, true);
const getIsArbitraryValue$1 = (value, testLabel, testValue) => {
  const result = arbitraryValueRegex$1.exec(value);
  if (result) {
    if (result[1]) {
      return testLabel(result[1]);
    }
    return testValue(result[2]);
  }
  return false;
};
const getIsArbitraryVariable$1 = (value, testLabel, shouldMatchNoLabel = false) => {
  const result = arbitraryVariableRegex$1.exec(value);
  if (result) {
    if (result[1]) {
      return testLabel(result[1]);
    }
    return shouldMatchNoLabel;
  }
  return false;
};
const isLabelPosition$1 = (label) => label === "position" || label === "percentage";
const isLabelImage$1 = (label) => label === "image" || label === "url";
const isLabelSize$1 = (label) => label === "length" || label === "size" || label === "bg-size";
const isLabelLength$1 = (label) => label === "length";
const isLabelNumber$1 = (label) => label === "number";
const isLabelFamilyName$1 = (label) => label === "family-name";
const isLabelWeight$1 = (label) => label === "number" || label === "weight";
const isLabelShadow$1 = (label) => label === "shadow";
const getDefaultConfig$1 = () => {
  const themeColor = fromTheme$1("color");
  const themeFont = fromTheme$1("font");
  const themeText = fromTheme$1("text");
  const themeFontWeight = fromTheme$1("font-weight");
  const themeTracking = fromTheme$1("tracking");
  const themeLeading = fromTheme$1("leading");
  const themeBreakpoint = fromTheme$1("breakpoint");
  const themeContainer = fromTheme$1("container");
  const themeSpacing = fromTheme$1("spacing");
  const themeRadius = fromTheme$1("radius");
  const themeShadow = fromTheme$1("shadow");
  const themeInsetShadow = fromTheme$1("inset-shadow");
  const themeTextShadow = fromTheme$1("text-shadow");
  const themeDropShadow = fromTheme$1("drop-shadow");
  const themeBlur = fromTheme$1("blur");
  const themePerspective = fromTheme$1("perspective");
  const themeAspect = fromTheme$1("aspect");
  const themeEase = fromTheme$1("ease");
  const themeAnimate = fromTheme$1("animate");
  const scaleBreak = () => ["auto", "avoid", "all", "avoid-page", "page", "left", "right", "column"];
  const scalePosition = () => [
    "center",
    "top",
    "bottom",
    "left",
    "right",
    "top-left",
    // Deprecated since Tailwind CSS v4.1.0, see https://github.com/tailwindlabs/tailwindcss/pull/17378
    "left-top",
    "top-right",
    // Deprecated since Tailwind CSS v4.1.0, see https://github.com/tailwindlabs/tailwindcss/pull/17378
    "right-top",
    "bottom-right",
    // Deprecated since Tailwind CSS v4.1.0, see https://github.com/tailwindlabs/tailwindcss/pull/17378
    "right-bottom",
    "bottom-left",
    // Deprecated since Tailwind CSS v4.1.0, see https://github.com/tailwindlabs/tailwindcss/pull/17378
    "left-bottom"
  ];
  const scalePositionWithArbitrary = () => [...scalePosition(), isArbitraryVariable$1, isArbitraryValue$1];
  const scaleOverflow = () => ["auto", "hidden", "clip", "visible", "scroll"];
  const scaleOverscroll = () => ["auto", "contain", "none"];
  const scaleUnambiguousSpacing = () => [isArbitraryVariable$1, isArbitraryValue$1, themeSpacing];
  const scaleInset = () => [isFraction$1, "full", "auto", ...scaleUnambiguousSpacing()];
  const scaleGridTemplateColsRows = () => [isInteger$1, "none", "subgrid", isArbitraryVariable$1, isArbitraryValue$1];
  const scaleGridColRowStartAndEnd = () => ["auto", {
    span: ["full", isInteger$1, isArbitraryVariable$1, isArbitraryValue$1]
  }, isInteger$1, isArbitraryVariable$1, isArbitraryValue$1];
  const scaleGridColRowStartOrEnd = () => [isInteger$1, "auto", isArbitraryVariable$1, isArbitraryValue$1];
  const scaleGridAutoColsRows = () => ["auto", "min", "max", "fr", isArbitraryVariable$1, isArbitraryValue$1];
  const scaleAlignPrimaryAxis = () => ["start", "end", "center", "between", "around", "evenly", "stretch", "baseline", "center-safe", "end-safe"];
  const scaleAlignSecondaryAxis = () => ["start", "end", "center", "stretch", "center-safe", "end-safe"];
  const scaleMargin = () => ["auto", ...scaleUnambiguousSpacing()];
  const scaleSizing = () => [isFraction$1, "auto", "full", "dvw", "dvh", "lvw", "lvh", "svw", "svh", "min", "max", "fit", ...scaleUnambiguousSpacing()];
  const scaleSizingInline = () => [themeContainer, isFraction$1, "screen", "full", "dvw", "lvw", "svw", "min", "max", "fit", ...scaleUnambiguousSpacing()];
  const scaleSizingBlock = () => [isFraction$1, "screen", "full", "lh", "dvh", "lvh", "svh", "min", "max", "fit", ...scaleUnambiguousSpacing()];
  const scaleColor = () => [themeColor, isArbitraryVariable$1, isArbitraryValue$1];
  const scaleBgPosition = () => [...scalePosition(), isArbitraryVariablePosition$1, isArbitraryPosition$1, {
    position: [isArbitraryVariable$1, isArbitraryValue$1]
  }];
  const scaleBgRepeat = () => ["no-repeat", {
    repeat: ["", "x", "y", "space", "round"]
  }];
  const scaleBgSize = () => ["auto", "cover", "contain", isArbitraryVariableSize$1, isArbitrarySize$1, {
    size: [isArbitraryVariable$1, isArbitraryValue$1]
  }];
  const scaleGradientStopPosition = () => [isPercent$1, isArbitraryVariableLength$1, isArbitraryLength$1];
  const scaleRadius = () => [
    // Deprecated since Tailwind CSS v4.0.0
    "",
    "none",
    "full",
    themeRadius,
    isArbitraryVariable$1,
    isArbitraryValue$1
  ];
  const scaleBorderWidth = () => ["", isNumber$1, isArbitraryVariableLength$1, isArbitraryLength$1];
  const scaleLineStyle = () => ["solid", "dashed", "dotted", "double"];
  const scaleBlendMode = () => ["normal", "multiply", "screen", "overlay", "darken", "lighten", "color-dodge", "color-burn", "hard-light", "soft-light", "difference", "exclusion", "hue", "saturation", "color", "luminosity"];
  const scaleMaskImagePosition = () => [isNumber$1, isPercent$1, isArbitraryVariablePosition$1, isArbitraryPosition$1];
  const scaleBlur = () => [
    // Deprecated since Tailwind CSS v4.0.0
    "",
    "none",
    themeBlur,
    isArbitraryVariable$1,
    isArbitraryValue$1
  ];
  const scaleRotate = () => ["none", isNumber$1, isArbitraryVariable$1, isArbitraryValue$1];
  const scaleScale = () => ["none", isNumber$1, isArbitraryVariable$1, isArbitraryValue$1];
  const scaleSkew = () => [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1];
  const scaleTranslate = () => [isFraction$1, "full", ...scaleUnambiguousSpacing()];
  return {
    cacheSize: 500,
    theme: {
      animate: ["spin", "ping", "pulse", "bounce"],
      aspect: ["video"],
      blur: [isTshirtSize$1],
      breakpoint: [isTshirtSize$1],
      color: [isAny$1],
      container: [isTshirtSize$1],
      "drop-shadow": [isTshirtSize$1],
      ease: ["in", "out", "in-out"],
      font: [isAnyNonArbitrary$1],
      "font-weight": ["thin", "extralight", "light", "normal", "medium", "semibold", "bold", "extrabold", "black"],
      "inset-shadow": [isTshirtSize$1],
      leading: ["none", "tight", "snug", "normal", "relaxed", "loose"],
      perspective: ["dramatic", "near", "normal", "midrange", "distant", "none"],
      radius: [isTshirtSize$1],
      shadow: [isTshirtSize$1],
      spacing: ["px", isNumber$1],
      text: [isTshirtSize$1],
      "text-shadow": [isTshirtSize$1],
      tracking: ["tighter", "tight", "normal", "wide", "wider", "widest"]
    },
    classGroups: {
      // --------------
      // --- Layout ---
      // --------------
      /**
       * Aspect Ratio
       * @see https://tailwindcss.com/docs/aspect-ratio
       */
      aspect: [{
        aspect: ["auto", "square", isFraction$1, isArbitraryValue$1, isArbitraryVariable$1, themeAspect]
      }],
      /**
       * Container
       * @see https://tailwindcss.com/docs/container
       * @deprecated since Tailwind CSS v4.0.0
       */
      container: ["container"],
      /**
       * Container Type
       * @see https://tailwindcss.com/docs/responsive-design#container-queries
       */
      "container-type": [{
        "@container": ["", "normal", "size", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Container Name
       * @see https://tailwindcss.com/docs/responsive-design#named-containers
       */
      "container-named": [isNamedContainerQuery$1],
      /**
       * Columns
       * @see https://tailwindcss.com/docs/columns
       */
      columns: [{
        columns: [isNumber$1, "auto", isArbitraryValue$1, isArbitraryVariable$1, themeContainer]
      }],
      /**
       * Break After
       * @see https://tailwindcss.com/docs/break-after
       */
      "break-after": [{
        "break-after": scaleBreak()
      }],
      /**
       * Break Before
       * @see https://tailwindcss.com/docs/break-before
       */
      "break-before": [{
        "break-before": scaleBreak()
      }],
      /**
       * Break Inside
       * @see https://tailwindcss.com/docs/break-inside
       */
      "break-inside": [{
        "break-inside": ["auto", "avoid", "avoid-page", "avoid-column"]
      }],
      /**
       * Box Decoration Break
       * @see https://tailwindcss.com/docs/box-decoration-break
       */
      "box-decoration": [{
        "box-decoration": ["slice", "clone"]
      }],
      /**
       * Box Sizing
       * @see https://tailwindcss.com/docs/box-sizing
       */
      box: [{
        box: ["border", "content"]
      }],
      /**
       * Display
       * @see https://tailwindcss.com/docs/display
       */
      display: ["block", "inline-block", "inline", "flex", "inline-flex", "table", "inline-table", "table-caption", "table-cell", "table-column", "table-column-group", "table-footer-group", "table-header-group", "table-row-group", "table-row", "flow-root", "grid", "inline-grid", "contents", "list-item", "hidden"],
      /**
       * Screen Reader Only
       * @see https://tailwindcss.com/docs/display#screen-reader-only
       */
      sr: ["sr-only", "not-sr-only"],
      /**
       * Floats
       * @see https://tailwindcss.com/docs/float
       */
      float: [{
        float: ["right", "left", "none", "start", "end"]
      }],
      /**
       * Clear
       * @see https://tailwindcss.com/docs/clear
       */
      clear: [{
        clear: ["left", "right", "both", "none", "start", "end"]
      }],
      /**
       * Isolation
       * @see https://tailwindcss.com/docs/isolation
       */
      isolation: ["isolate", "isolation-auto"],
      /**
       * Object Fit
       * @see https://tailwindcss.com/docs/object-fit
       */
      "object-fit": [{
        object: ["contain", "cover", "fill", "none", "scale-down"]
      }],
      /**
       * Object Position
       * @see https://tailwindcss.com/docs/object-position
       */
      "object-position": [{
        object: scalePositionWithArbitrary()
      }],
      /**
       * Overflow
       * @see https://tailwindcss.com/docs/overflow
       */
      overflow: [{
        overflow: scaleOverflow()
      }],
      /**
       * Overflow X
       * @see https://tailwindcss.com/docs/overflow
       */
      "overflow-x": [{
        "overflow-x": scaleOverflow()
      }],
      /**
       * Overflow Y
       * @see https://tailwindcss.com/docs/overflow
       */
      "overflow-y": [{
        "overflow-y": scaleOverflow()
      }],
      /**
       * Overscroll Behavior
       * @see https://tailwindcss.com/docs/overscroll-behavior
       */
      overscroll: [{
        overscroll: scaleOverscroll()
      }],
      /**
       * Overscroll Behavior X
       * @see https://tailwindcss.com/docs/overscroll-behavior
       */
      "overscroll-x": [{
        "overscroll-x": scaleOverscroll()
      }],
      /**
       * Overscroll Behavior Y
       * @see https://tailwindcss.com/docs/overscroll-behavior
       */
      "overscroll-y": [{
        "overscroll-y": scaleOverscroll()
      }],
      /**
       * Position
       * @see https://tailwindcss.com/docs/position
       */
      position: ["static", "fixed", "absolute", "relative", "sticky"],
      /**
       * Inset
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      inset: [{
        inset: scaleInset()
      }],
      /**
       * Inset Inline
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      "inset-x": [{
        "inset-x": scaleInset()
      }],
      /**
       * Inset Block
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      "inset-y": [{
        "inset-y": scaleInset()
      }],
      /**
       * Inset Inline Start
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       * @todo class group will be renamed to `inset-s` in next major release
       */
      start: [{
        "inset-s": scaleInset(),
        /**
         * @deprecated since Tailwind CSS v4.2.0 in favor of `inset-s-*` utilities.
         * @see https://github.com/tailwindlabs/tailwindcss/pull/19613
         */
        start: scaleInset()
      }],
      /**
       * Inset Inline End
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       * @todo class group will be renamed to `inset-e` in next major release
       */
      end: [{
        "inset-e": scaleInset(),
        /**
         * @deprecated since Tailwind CSS v4.2.0 in favor of `inset-e-*` utilities.
         * @see https://github.com/tailwindlabs/tailwindcss/pull/19613
         */
        end: scaleInset()
      }],
      /**
       * Inset Block Start
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      "inset-bs": [{
        "inset-bs": scaleInset()
      }],
      /**
       * Inset Block End
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      "inset-be": [{
        "inset-be": scaleInset()
      }],
      /**
       * Top
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      top: [{
        top: scaleInset()
      }],
      /**
       * Right
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      right: [{
        right: scaleInset()
      }],
      /**
       * Bottom
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      bottom: [{
        bottom: scaleInset()
      }],
      /**
       * Left
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      left: [{
        left: scaleInset()
      }],
      /**
       * Visibility
       * @see https://tailwindcss.com/docs/visibility
       */
      visibility: ["visible", "invisible", "collapse"],
      /**
       * Z-Index
       * @see https://tailwindcss.com/docs/z-index
       */
      z: [{
        z: [isInteger$1, "auto", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      // ------------------------
      // --- Flexbox and Grid ---
      // ------------------------
      /**
       * Flex Basis
       * @see https://tailwindcss.com/docs/flex-basis
       */
      basis: [{
        basis: [isFraction$1, "full", "auto", themeContainer, ...scaleUnambiguousSpacing()]
      }],
      /**
       * Flex Direction
       * @see https://tailwindcss.com/docs/flex-direction
       */
      "flex-direction": [{
        flex: ["row", "row-reverse", "col", "col-reverse"]
      }],
      /**
       * Flex Wrap
       * @see https://tailwindcss.com/docs/flex-wrap
       */
      "flex-wrap": [{
        flex: ["nowrap", "wrap", "wrap-reverse"]
      }],
      /**
       * Flex
       * @see https://tailwindcss.com/docs/flex
       */
      flex: [{
        flex: [isNumber$1, isFraction$1, "auto", "initial", "none", isArbitraryValue$1]
      }],
      /**
       * Flex Grow
       * @see https://tailwindcss.com/docs/flex-grow
       */
      grow: [{
        grow: ["", isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Flex Shrink
       * @see https://tailwindcss.com/docs/flex-shrink
       */
      shrink: [{
        shrink: ["", isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Order
       * @see https://tailwindcss.com/docs/order
       */
      order: [{
        order: [isInteger$1, "first", "last", "none", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Grid Template Columns
       * @see https://tailwindcss.com/docs/grid-template-columns
       */
      "grid-cols": [{
        "grid-cols": scaleGridTemplateColsRows()
      }],
      /**
       * Grid Column Start / End
       * @see https://tailwindcss.com/docs/grid-column
       */
      "col-start-end": [{
        col: scaleGridColRowStartAndEnd()
      }],
      /**
       * Grid Column Start
       * @see https://tailwindcss.com/docs/grid-column
       */
      "col-start": [{
        "col-start": scaleGridColRowStartOrEnd()
      }],
      /**
       * Grid Column End
       * @see https://tailwindcss.com/docs/grid-column
       */
      "col-end": [{
        "col-end": scaleGridColRowStartOrEnd()
      }],
      /**
       * Grid Template Rows
       * @see https://tailwindcss.com/docs/grid-template-rows
       */
      "grid-rows": [{
        "grid-rows": scaleGridTemplateColsRows()
      }],
      /**
       * Grid Row Start / End
       * @see https://tailwindcss.com/docs/grid-row
       */
      "row-start-end": [{
        row: scaleGridColRowStartAndEnd()
      }],
      /**
       * Grid Row Start
       * @see https://tailwindcss.com/docs/grid-row
       */
      "row-start": [{
        "row-start": scaleGridColRowStartOrEnd()
      }],
      /**
       * Grid Row End
       * @see https://tailwindcss.com/docs/grid-row
       */
      "row-end": [{
        "row-end": scaleGridColRowStartOrEnd()
      }],
      /**
       * Grid Auto Flow
       * @see https://tailwindcss.com/docs/grid-auto-flow
       */
      "grid-flow": [{
        "grid-flow": ["row", "col", "dense", "row-dense", "col-dense"]
      }],
      /**
       * Grid Auto Columns
       * @see https://tailwindcss.com/docs/grid-auto-columns
       */
      "auto-cols": [{
        "auto-cols": scaleGridAutoColsRows()
      }],
      /**
       * Grid Auto Rows
       * @see https://tailwindcss.com/docs/grid-auto-rows
       */
      "auto-rows": [{
        "auto-rows": scaleGridAutoColsRows()
      }],
      /**
       * Gap
       * @see https://tailwindcss.com/docs/gap
       */
      gap: [{
        gap: scaleUnambiguousSpacing()
      }],
      /**
       * Gap X
       * @see https://tailwindcss.com/docs/gap
       */
      "gap-x": [{
        "gap-x": scaleUnambiguousSpacing()
      }],
      /**
       * Gap Y
       * @see https://tailwindcss.com/docs/gap
       */
      "gap-y": [{
        "gap-y": scaleUnambiguousSpacing()
      }],
      /**
       * Justify Content
       * @see https://tailwindcss.com/docs/justify-content
       */
      "justify-content": [{
        justify: [...scaleAlignPrimaryAxis(), "normal"]
      }],
      /**
       * Justify Items
       * @see https://tailwindcss.com/docs/justify-items
       */
      "justify-items": [{
        "justify-items": [...scaleAlignSecondaryAxis(), "normal"]
      }],
      /**
       * Justify Self
       * @see https://tailwindcss.com/docs/justify-self
       */
      "justify-self": [{
        "justify-self": ["auto", ...scaleAlignSecondaryAxis()]
      }],
      /**
       * Align Content
       * @see https://tailwindcss.com/docs/align-content
       */
      "align-content": [{
        content: ["normal", ...scaleAlignPrimaryAxis()]
      }],
      /**
       * Align Items
       * @see https://tailwindcss.com/docs/align-items
       */
      "align-items": [{
        items: [...scaleAlignSecondaryAxis(), {
          baseline: ["", "last"]
        }]
      }],
      /**
       * Align Self
       * @see https://tailwindcss.com/docs/align-self
       */
      "align-self": [{
        self: ["auto", ...scaleAlignSecondaryAxis(), {
          baseline: ["", "last"]
        }]
      }],
      /**
       * Place Content
       * @see https://tailwindcss.com/docs/place-content
       */
      "place-content": [{
        "place-content": scaleAlignPrimaryAxis()
      }],
      /**
       * Place Items
       * @see https://tailwindcss.com/docs/place-items
       */
      "place-items": [{
        "place-items": [...scaleAlignSecondaryAxis(), "baseline"]
      }],
      /**
       * Place Self
       * @see https://tailwindcss.com/docs/place-self
       */
      "place-self": [{
        "place-self": ["auto", ...scaleAlignSecondaryAxis()]
      }],
      // Spacing
      /**
       * Padding
       * @see https://tailwindcss.com/docs/padding
       */
      p: [{
        p: scaleUnambiguousSpacing()
      }],
      /**
       * Padding Inline
       * @see https://tailwindcss.com/docs/padding
       */
      px: [{
        px: scaleUnambiguousSpacing()
      }],
      /**
       * Padding Block
       * @see https://tailwindcss.com/docs/padding
       */
      py: [{
        py: scaleUnambiguousSpacing()
      }],
      /**
       * Padding Inline Start
       * @see https://tailwindcss.com/docs/padding
       */
      ps: [{
        ps: scaleUnambiguousSpacing()
      }],
      /**
       * Padding Inline End
       * @see https://tailwindcss.com/docs/padding
       */
      pe: [{
        pe: scaleUnambiguousSpacing()
      }],
      /**
       * Padding Block Start
       * @see https://tailwindcss.com/docs/padding
       */
      pbs: [{
        pbs: scaleUnambiguousSpacing()
      }],
      /**
       * Padding Block End
       * @see https://tailwindcss.com/docs/padding
       */
      pbe: [{
        pbe: scaleUnambiguousSpacing()
      }],
      /**
       * Padding Top
       * @see https://tailwindcss.com/docs/padding
       */
      pt: [{
        pt: scaleUnambiguousSpacing()
      }],
      /**
       * Padding Right
       * @see https://tailwindcss.com/docs/padding
       */
      pr: [{
        pr: scaleUnambiguousSpacing()
      }],
      /**
       * Padding Bottom
       * @see https://tailwindcss.com/docs/padding
       */
      pb: [{
        pb: scaleUnambiguousSpacing()
      }],
      /**
       * Padding Left
       * @see https://tailwindcss.com/docs/padding
       */
      pl: [{
        pl: scaleUnambiguousSpacing()
      }],
      /**
       * Margin
       * @see https://tailwindcss.com/docs/margin
       */
      m: [{
        m: scaleMargin()
      }],
      /**
       * Margin Inline
       * @see https://tailwindcss.com/docs/margin
       */
      mx: [{
        mx: scaleMargin()
      }],
      /**
       * Margin Block
       * @see https://tailwindcss.com/docs/margin
       */
      my: [{
        my: scaleMargin()
      }],
      /**
       * Margin Inline Start
       * @see https://tailwindcss.com/docs/margin
       */
      ms: [{
        ms: scaleMargin()
      }],
      /**
       * Margin Inline End
       * @see https://tailwindcss.com/docs/margin
       */
      me: [{
        me: scaleMargin()
      }],
      /**
       * Margin Block Start
       * @see https://tailwindcss.com/docs/margin
       */
      mbs: [{
        mbs: scaleMargin()
      }],
      /**
       * Margin Block End
       * @see https://tailwindcss.com/docs/margin
       */
      mbe: [{
        mbe: scaleMargin()
      }],
      /**
       * Margin Top
       * @see https://tailwindcss.com/docs/margin
       */
      mt: [{
        mt: scaleMargin()
      }],
      /**
       * Margin Right
       * @see https://tailwindcss.com/docs/margin
       */
      mr: [{
        mr: scaleMargin()
      }],
      /**
       * Margin Bottom
       * @see https://tailwindcss.com/docs/margin
       */
      mb: [{
        mb: scaleMargin()
      }],
      /**
       * Margin Left
       * @see https://tailwindcss.com/docs/margin
       */
      ml: [{
        ml: scaleMargin()
      }],
      /**
       * Space Between X
       * @see https://tailwindcss.com/docs/margin#adding-space-between-children
       */
      "space-x": [{
        "space-x": scaleUnambiguousSpacing()
      }],
      /**
       * Space Between X Reverse
       * @see https://tailwindcss.com/docs/margin#adding-space-between-children
       */
      "space-x-reverse": ["space-x-reverse"],
      /**
       * Space Between Y
       * @see https://tailwindcss.com/docs/margin#adding-space-between-children
       */
      "space-y": [{
        "space-y": scaleUnambiguousSpacing()
      }],
      /**
       * Space Between Y Reverse
       * @see https://tailwindcss.com/docs/margin#adding-space-between-children
       */
      "space-y-reverse": ["space-y-reverse"],
      // --------------
      // --- Sizing ---
      // --------------
      /**
       * Size
       * @see https://tailwindcss.com/docs/width#setting-both-width-and-height
       */
      size: [{
        size: scaleSizing()
      }],
      /**
       * Inline Size
       * @see https://tailwindcss.com/docs/inline-size
       */
      "inline-size": [{
        inline: ["auto", ...scaleSizingInline()]
      }],
      /**
       * Min-Inline Size
       * @see https://tailwindcss.com/docs/min-inline-size
       */
      "min-inline-size": [{
        "min-inline": ["auto", ...scaleSizingInline()]
      }],
      /**
       * Max-Inline Size
       * @see https://tailwindcss.com/docs/max-inline-size
       */
      "max-inline-size": [{
        "max-inline": ["none", ...scaleSizingInline()]
      }],
      /**
       * Block Size
       * @see https://tailwindcss.com/docs/block-size
       */
      "block-size": [{
        block: ["auto", ...scaleSizingBlock()]
      }],
      /**
       * Min-Block Size
       * @see https://tailwindcss.com/docs/min-block-size
       */
      "min-block-size": [{
        "min-block": ["auto", ...scaleSizingBlock()]
      }],
      /**
       * Max-Block Size
       * @see https://tailwindcss.com/docs/max-block-size
       */
      "max-block-size": [{
        "max-block": ["none", ...scaleSizingBlock()]
      }],
      /**
       * Width
       * @see https://tailwindcss.com/docs/width
       */
      w: [{
        w: [themeContainer, "screen", ...scaleSizing()]
      }],
      /**
       * Min-Width
       * @see https://tailwindcss.com/docs/min-width
       */
      "min-w": [{
        "min-w": [
          themeContainer,
          "screen",
          /** Deprecated. @see https://github.com/tailwindlabs/tailwindcss.com/issues/2027#issuecomment-2620152757 */
          "none",
          ...scaleSizing()
        ]
      }],
      /**
       * Max-Width
       * @see https://tailwindcss.com/docs/max-width
       */
      "max-w": [{
        "max-w": [
          themeContainer,
          "screen",
          "none",
          /** Deprecated since Tailwind CSS v4.0.0. @see https://github.com/tailwindlabs/tailwindcss.com/issues/2027#issuecomment-2620152757 */
          "prose",
          /** Deprecated since Tailwind CSS v4.0.0. @see https://github.com/tailwindlabs/tailwindcss.com/issues/2027#issuecomment-2620152757 */
          {
            screen: [themeBreakpoint]
          },
          ...scaleSizing()
        ]
      }],
      /**
       * Height
       * @see https://tailwindcss.com/docs/height
       */
      h: [{
        h: ["screen", "lh", ...scaleSizing()]
      }],
      /**
       * Min-Height
       * @see https://tailwindcss.com/docs/min-height
       */
      "min-h": [{
        "min-h": ["screen", "lh", "none", ...scaleSizing()]
      }],
      /**
       * Max-Height
       * @see https://tailwindcss.com/docs/max-height
       */
      "max-h": [{
        "max-h": ["screen", "lh", "none", ...scaleSizing()]
      }],
      // ------------------
      // --- Typography ---
      // ------------------
      /**
       * Font Size
       * @see https://tailwindcss.com/docs/font-size
       */
      "font-size": [{
        text: ["base", themeText, isArbitraryVariableLength$1, isArbitraryLength$1]
      }],
      /**
       * Font Smoothing
       * @see https://tailwindcss.com/docs/font-smoothing
       */
      "font-smoothing": ["antialiased", "subpixel-antialiased"],
      /**
       * Font Style
       * @see https://tailwindcss.com/docs/font-style
       */
      "font-style": ["italic", "not-italic"],
      /**
       * Font Weight
       * @see https://tailwindcss.com/docs/font-weight
       */
      "font-weight": [{
        font: [themeFontWeight, isArbitraryVariableWeight$1, isArbitraryWeight$1]
      }],
      /**
       * Font Stretch
       * @see https://tailwindcss.com/docs/font-stretch
       */
      "font-stretch": [{
        "font-stretch": ["ultra-condensed", "extra-condensed", "condensed", "semi-condensed", "normal", "semi-expanded", "expanded", "extra-expanded", "ultra-expanded", isPercent$1, isArbitraryValue$1]
      }],
      /**
       * Font Family
       * @see https://tailwindcss.com/docs/font-family
       */
      "font-family": [{
        font: [isArbitraryVariableFamilyName$1, isArbitraryFamilyName$1, themeFont]
      }],
      /**
       * Font Feature Settings
       * @see https://tailwindcss.com/docs/font-feature-settings
       */
      "font-features": [{
        "font-features": [isArbitraryValue$1]
      }],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-normal": ["normal-nums"],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-ordinal": ["ordinal"],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-slashed-zero": ["slashed-zero"],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-figure": ["lining-nums", "oldstyle-nums"],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-spacing": ["proportional-nums", "tabular-nums"],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-fraction": ["diagonal-fractions", "stacked-fractions"],
      /**
       * Letter Spacing
       * @see https://tailwindcss.com/docs/letter-spacing
       */
      tracking: [{
        tracking: [themeTracking, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Line Clamp
       * @see https://tailwindcss.com/docs/line-clamp
       */
      "line-clamp": [{
        "line-clamp": [isNumber$1, "none", isArbitraryVariable$1, isArbitraryNumber$1]
      }],
      /**
       * Line Height
       * @see https://tailwindcss.com/docs/line-height
       */
      leading: [{
        leading: [
          "none",
          /** Deprecated since Tailwind CSS v4.0.0. @see https://github.com/tailwindlabs/tailwindcss.com/issues/2027#issuecomment-2620152757 */
          themeLeading,
          ...scaleUnambiguousSpacing()
        ]
      }],
      /**
       * List Style Image
       * @see https://tailwindcss.com/docs/list-style-image
       */
      "list-image": [{
        "list-image": ["none", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * List Style Position
       * @see https://tailwindcss.com/docs/list-style-position
       */
      "list-style-position": [{
        list: ["inside", "outside"]
      }],
      /**
       * List Style Type
       * @see https://tailwindcss.com/docs/list-style-type
       */
      "list-style-type": [{
        list: ["disc", "decimal", "none", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Text Alignment
       * @see https://tailwindcss.com/docs/text-align
       */
      "text-alignment": [{
        text: ["left", "center", "right", "justify", "start", "end"]
      }],
      /**
       * Placeholder Color
       * @deprecated since Tailwind CSS v3.0.0
       * @see https://v3.tailwindcss.com/docs/placeholder-color
       */
      "placeholder-color": [{
        placeholder: scaleColor()
      }],
      /**
       * Text Color
       * @see https://tailwindcss.com/docs/text-color
       */
      "text-color": [{
        text: scaleColor()
      }],
      /**
       * Text Decoration
       * @see https://tailwindcss.com/docs/text-decoration
       */
      "text-decoration": ["underline", "overline", "line-through", "no-underline"],
      /**
       * Text Decoration Style
       * @see https://tailwindcss.com/docs/text-decoration-style
       */
      "text-decoration-style": [{
        decoration: [...scaleLineStyle(), "wavy"]
      }],
      /**
       * Text Decoration Thickness
       * @see https://tailwindcss.com/docs/text-decoration-thickness
       */
      "text-decoration-thickness": [{
        decoration: [isNumber$1, "from-font", "auto", isArbitraryVariable$1, isArbitraryLength$1]
      }],
      /**
       * Text Decoration Color
       * @see https://tailwindcss.com/docs/text-decoration-color
       */
      "text-decoration-color": [{
        decoration: scaleColor()
      }],
      /**
       * Text Underline Offset
       * @see https://tailwindcss.com/docs/text-underline-offset
       */
      "underline-offset": [{
        "underline-offset": [isNumber$1, "auto", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Text Transform
       * @see https://tailwindcss.com/docs/text-transform
       */
      "text-transform": ["uppercase", "lowercase", "capitalize", "normal-case"],
      /**
       * Text Overflow
       * @see https://tailwindcss.com/docs/text-overflow
       */
      "text-overflow": ["truncate", "text-ellipsis", "text-clip"],
      /**
       * Text Wrap
       * @see https://tailwindcss.com/docs/text-wrap
       */
      "text-wrap": [{
        text: ["wrap", "nowrap", "balance", "pretty"]
      }],
      /**
       * Text Indent
       * @see https://tailwindcss.com/docs/text-indent
       */
      indent: [{
        indent: scaleUnambiguousSpacing()
      }],
      /**
       * Tab Size
       * @see https://tailwindcss.com/docs/tab-size
       */
      "tab-size": [{
        tab: [isInteger$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Vertical Alignment
       * @see https://tailwindcss.com/docs/vertical-align
       */
      "vertical-align": [{
        align: ["baseline", "top", "middle", "bottom", "text-top", "text-bottom", "sub", "super", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Whitespace
       * @see https://tailwindcss.com/docs/whitespace
       */
      whitespace: [{
        whitespace: ["normal", "nowrap", "pre", "pre-line", "pre-wrap", "break-spaces"]
      }],
      /**
       * Word Break
       * @see https://tailwindcss.com/docs/word-break
       */
      break: [{
        break: ["normal", "words", "all", "keep"]
      }],
      /**
       * Overflow Wrap
       * @see https://tailwindcss.com/docs/overflow-wrap
       */
      wrap: [{
        wrap: ["break-word", "anywhere", "normal"]
      }],
      /**
       * Hyphens
       * @see https://tailwindcss.com/docs/hyphens
       */
      hyphens: [{
        hyphens: ["none", "manual", "auto"]
      }],
      /**
       * Content
       * @see https://tailwindcss.com/docs/content
       */
      content: [{
        content: ["none", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      // -------------------
      // --- Backgrounds ---
      // -------------------
      /**
       * Background Attachment
       * @see https://tailwindcss.com/docs/background-attachment
       */
      "bg-attachment": [{
        bg: ["fixed", "local", "scroll"]
      }],
      /**
       * Background Clip
       * @see https://tailwindcss.com/docs/background-clip
       */
      "bg-clip": [{
        "bg-clip": ["border", "padding", "content", "text"]
      }],
      /**
       * Background Origin
       * @see https://tailwindcss.com/docs/background-origin
       */
      "bg-origin": [{
        "bg-origin": ["border", "padding", "content"]
      }],
      /**
       * Background Position
       * @see https://tailwindcss.com/docs/background-position
       */
      "bg-position": [{
        bg: scaleBgPosition()
      }],
      /**
       * Background Repeat
       * @see https://tailwindcss.com/docs/background-repeat
       */
      "bg-repeat": [{
        bg: scaleBgRepeat()
      }],
      /**
       * Background Size
       * @see https://tailwindcss.com/docs/background-size
       */
      "bg-size": [{
        bg: scaleBgSize()
      }],
      /**
       * Background Image
       * @see https://tailwindcss.com/docs/background-image
       */
      "bg-image": [{
        bg: ["none", {
          linear: [{
            to: ["t", "tr", "r", "br", "b", "bl", "l", "tl"]
          }, isInteger$1, isArbitraryVariable$1, isArbitraryValue$1],
          radial: ["", isArbitraryVariable$1, isArbitraryValue$1],
          conic: ["", isInteger$1, isArbitraryVariable$1, isArbitraryValue$1]
        }, isArbitraryVariableImage$1, isArbitraryImage$1]
      }],
      /**
       * Background Color
       * @see https://tailwindcss.com/docs/background-color
       */
      "bg-color": [{
        bg: scaleColor()
      }],
      /**
       * Gradient Color Stops From Position
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-from-pos": [{
        from: scaleGradientStopPosition()
      }],
      /**
       * Gradient Color Stops Via Position
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-via-pos": [{
        via: scaleGradientStopPosition()
      }],
      /**
       * Gradient Color Stops To Position
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-to-pos": [{
        to: scaleGradientStopPosition()
      }],
      /**
       * Gradient Color Stops From
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-from": [{
        from: scaleColor()
      }],
      /**
       * Gradient Color Stops Via
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-via": [{
        via: scaleColor()
      }],
      /**
       * Gradient Color Stops To
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-to": [{
        to: scaleColor()
      }],
      // ---------------
      // --- Borders ---
      // ---------------
      /**
       * Border Radius
       * @see https://tailwindcss.com/docs/border-radius
       */
      rounded: [{
        rounded: scaleRadius()
      }],
      /**
       * Border Radius Start
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-s": [{
        "rounded-s": scaleRadius()
      }],
      /**
       * Border Radius End
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-e": [{
        "rounded-e": scaleRadius()
      }],
      /**
       * Border Radius Top
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-t": [{
        "rounded-t": scaleRadius()
      }],
      /**
       * Border Radius Right
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-r": [{
        "rounded-r": scaleRadius()
      }],
      /**
       * Border Radius Bottom
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-b": [{
        "rounded-b": scaleRadius()
      }],
      /**
       * Border Radius Left
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-l": [{
        "rounded-l": scaleRadius()
      }],
      /**
       * Border Radius Start Start
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-ss": [{
        "rounded-ss": scaleRadius()
      }],
      /**
       * Border Radius Start End
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-se": [{
        "rounded-se": scaleRadius()
      }],
      /**
       * Border Radius End End
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-ee": [{
        "rounded-ee": scaleRadius()
      }],
      /**
       * Border Radius End Start
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-es": [{
        "rounded-es": scaleRadius()
      }],
      /**
       * Border Radius Top Left
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-tl": [{
        "rounded-tl": scaleRadius()
      }],
      /**
       * Border Radius Top Right
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-tr": [{
        "rounded-tr": scaleRadius()
      }],
      /**
       * Border Radius Bottom Right
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-br": [{
        "rounded-br": scaleRadius()
      }],
      /**
       * Border Radius Bottom Left
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-bl": [{
        "rounded-bl": scaleRadius()
      }],
      /**
       * Border Width
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w": [{
        border: scaleBorderWidth()
      }],
      /**
       * Border Width Inline
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-x": [{
        "border-x": scaleBorderWidth()
      }],
      /**
       * Border Width Block
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-y": [{
        "border-y": scaleBorderWidth()
      }],
      /**
       * Border Width Inline Start
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-s": [{
        "border-s": scaleBorderWidth()
      }],
      /**
       * Border Width Inline End
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-e": [{
        "border-e": scaleBorderWidth()
      }],
      /**
       * Border Width Block Start
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-bs": [{
        "border-bs": scaleBorderWidth()
      }],
      /**
       * Border Width Block End
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-be": [{
        "border-be": scaleBorderWidth()
      }],
      /**
       * Border Width Top
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-t": [{
        "border-t": scaleBorderWidth()
      }],
      /**
       * Border Width Right
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-r": [{
        "border-r": scaleBorderWidth()
      }],
      /**
       * Border Width Bottom
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-b": [{
        "border-b": scaleBorderWidth()
      }],
      /**
       * Border Width Left
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-l": [{
        "border-l": scaleBorderWidth()
      }],
      /**
       * Divide Width X
       * @see https://tailwindcss.com/docs/border-width#between-children
       */
      "divide-x": [{
        "divide-x": scaleBorderWidth()
      }],
      /**
       * Divide Width X Reverse
       * @see https://tailwindcss.com/docs/border-width#between-children
       */
      "divide-x-reverse": ["divide-x-reverse"],
      /**
       * Divide Width Y
       * @see https://tailwindcss.com/docs/border-width#between-children
       */
      "divide-y": [{
        "divide-y": scaleBorderWidth()
      }],
      /**
       * Divide Width Y Reverse
       * @see https://tailwindcss.com/docs/border-width#between-children
       */
      "divide-y-reverse": ["divide-y-reverse"],
      /**
       * Border Style
       * @see https://tailwindcss.com/docs/border-style
       */
      "border-style": [{
        border: [...scaleLineStyle(), "hidden", "none"]
      }],
      /**
       * Divide Style
       * @see https://tailwindcss.com/docs/border-style#setting-the-divider-style
       */
      "divide-style": [{
        divide: [...scaleLineStyle(), "hidden", "none"]
      }],
      /**
       * Border Color
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color": [{
        border: scaleColor()
      }],
      /**
       * Border Color Inline
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-x": [{
        "border-x": scaleColor()
      }],
      /**
       * Border Color Block
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-y": [{
        "border-y": scaleColor()
      }],
      /**
       * Border Color Inline Start
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-s": [{
        "border-s": scaleColor()
      }],
      /**
       * Border Color Inline End
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-e": [{
        "border-e": scaleColor()
      }],
      /**
       * Border Color Block Start
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-bs": [{
        "border-bs": scaleColor()
      }],
      /**
       * Border Color Block End
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-be": [{
        "border-be": scaleColor()
      }],
      /**
       * Border Color Top
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-t": [{
        "border-t": scaleColor()
      }],
      /**
       * Border Color Right
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-r": [{
        "border-r": scaleColor()
      }],
      /**
       * Border Color Bottom
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-b": [{
        "border-b": scaleColor()
      }],
      /**
       * Border Color Left
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-l": [{
        "border-l": scaleColor()
      }],
      /**
       * Divide Color
       * @see https://tailwindcss.com/docs/divide-color
       */
      "divide-color": [{
        divide: scaleColor()
      }],
      /**
       * Outline Style
       * @see https://tailwindcss.com/docs/outline-style
       */
      "outline-style": [{
        outline: [...scaleLineStyle(), "none", "hidden"]
      }],
      /**
       * Outline Offset
       * @see https://tailwindcss.com/docs/outline-offset
       */
      "outline-offset": [{
        "outline-offset": [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Outline Width
       * @see https://tailwindcss.com/docs/outline-width
       */
      "outline-w": [{
        outline: ["", isNumber$1, isArbitraryVariableLength$1, isArbitraryLength$1]
      }],
      /**
       * Outline Color
       * @see https://tailwindcss.com/docs/outline-color
       */
      "outline-color": [{
        outline: scaleColor()
      }],
      // ---------------
      // --- Effects ---
      // ---------------
      /**
       * Box Shadow
       * @see https://tailwindcss.com/docs/box-shadow
       */
      shadow: [{
        shadow: [
          // Deprecated since Tailwind CSS v4.0.0
          "",
          // Deprecated since Tailwind CSS v4.0.0
          "inner",
          "none",
          themeShadow,
          isArbitraryVariableShadow$1,
          isArbitraryShadow$1
        ]
      }],
      /**
       * Box Shadow Color
       * @see https://tailwindcss.com/docs/box-shadow#setting-the-shadow-color
       */
      "shadow-color": [{
        shadow: scaleColor()
      }],
      /**
       * Inset Box Shadow
       * @see https://tailwindcss.com/docs/box-shadow#adding-an-inset-shadow
       */
      "inset-shadow": [{
        "inset-shadow": ["none", themeInsetShadow, isArbitraryVariableShadow$1, isArbitraryShadow$1]
      }],
      /**
       * Inset Box Shadow Color
       * @see https://tailwindcss.com/docs/box-shadow#setting-the-inset-shadow-color
       */
      "inset-shadow-color": [{
        "inset-shadow": scaleColor()
      }],
      /**
       * Ring Width
       * @see https://tailwindcss.com/docs/box-shadow#adding-a-ring
       */
      "ring-w": [{
        ring: scaleBorderWidth()
      }],
      /**
       * Ring Width Inset
       * @see https://v3.tailwindcss.com/docs/ring-width#inset-rings
       * @deprecated since Tailwind CSS v4.0.0
       * @see https://github.com/tailwindlabs/tailwindcss/blob/v4.0.0/packages/tailwindcss/src/utilities.ts#L4158
       */
      "ring-w-inset": ["ring-inset"],
      /**
       * Ring Color
       * @see https://tailwindcss.com/docs/box-shadow#setting-the-ring-color
       */
      "ring-color": [{
        ring: scaleColor()
      }],
      /**
       * Ring Offset Width
       * @see https://v3.tailwindcss.com/docs/ring-offset-width
       * @deprecated since Tailwind CSS v4.0.0
       * @see https://github.com/tailwindlabs/tailwindcss/blob/v4.0.0/packages/tailwindcss/src/utilities.ts#L4158
       */
      "ring-offset-w": [{
        "ring-offset": [isNumber$1, isArbitraryLength$1]
      }],
      /**
       * Ring Offset Color
       * @see https://v3.tailwindcss.com/docs/ring-offset-color
       * @deprecated since Tailwind CSS v4.0.0
       * @see https://github.com/tailwindlabs/tailwindcss/blob/v4.0.0/packages/tailwindcss/src/utilities.ts#L4158
       */
      "ring-offset-color": [{
        "ring-offset": scaleColor()
      }],
      /**
       * Inset Ring Width
       * @see https://tailwindcss.com/docs/box-shadow#adding-an-inset-ring
       */
      "inset-ring-w": [{
        "inset-ring": scaleBorderWidth()
      }],
      /**
       * Inset Ring Color
       * @see https://tailwindcss.com/docs/box-shadow#setting-the-inset-ring-color
       */
      "inset-ring-color": [{
        "inset-ring": scaleColor()
      }],
      /**
       * Text Shadow
       * @see https://tailwindcss.com/docs/text-shadow
       */
      "text-shadow": [{
        "text-shadow": ["none", themeTextShadow, isArbitraryVariableShadow$1, isArbitraryShadow$1]
      }],
      /**
       * Text Shadow Color
       * @see https://tailwindcss.com/docs/text-shadow#setting-the-shadow-color
       */
      "text-shadow-color": [{
        "text-shadow": scaleColor()
      }],
      /**
       * Opacity
       * @see https://tailwindcss.com/docs/opacity
       */
      opacity: [{
        opacity: [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Mix Blend Mode
       * @see https://tailwindcss.com/docs/mix-blend-mode
       */
      "mix-blend": [{
        "mix-blend": [...scaleBlendMode(), "plus-darker", "plus-lighter"]
      }],
      /**
       * Background Blend Mode
       * @see https://tailwindcss.com/docs/background-blend-mode
       */
      "bg-blend": [{
        "bg-blend": scaleBlendMode()
      }],
      /**
       * Mask Clip
       * @see https://tailwindcss.com/docs/mask-clip
       */
      "mask-clip": [{
        "mask-clip": ["border", "padding", "content", "fill", "stroke", "view"]
      }, "mask-no-clip"],
      /**
       * Mask Composite
       * @see https://tailwindcss.com/docs/mask-composite
       */
      "mask-composite": [{
        mask: ["add", "subtract", "intersect", "exclude"]
      }],
      /**
       * Mask Image
       * @see https://tailwindcss.com/docs/mask-image
       */
      "mask-image-linear-pos": [{
        "mask-linear": [isNumber$1]
      }],
      "mask-image-linear-from-pos": [{
        "mask-linear-from": scaleMaskImagePosition()
      }],
      "mask-image-linear-to-pos": [{
        "mask-linear-to": scaleMaskImagePosition()
      }],
      "mask-image-linear-from-color": [{
        "mask-linear-from": scaleColor()
      }],
      "mask-image-linear-to-color": [{
        "mask-linear-to": scaleColor()
      }],
      "mask-image-t-from-pos": [{
        "mask-t-from": scaleMaskImagePosition()
      }],
      "mask-image-t-to-pos": [{
        "mask-t-to": scaleMaskImagePosition()
      }],
      "mask-image-t-from-color": [{
        "mask-t-from": scaleColor()
      }],
      "mask-image-t-to-color": [{
        "mask-t-to": scaleColor()
      }],
      "mask-image-r-from-pos": [{
        "mask-r-from": scaleMaskImagePosition()
      }],
      "mask-image-r-to-pos": [{
        "mask-r-to": scaleMaskImagePosition()
      }],
      "mask-image-r-from-color": [{
        "mask-r-from": scaleColor()
      }],
      "mask-image-r-to-color": [{
        "mask-r-to": scaleColor()
      }],
      "mask-image-b-from-pos": [{
        "mask-b-from": scaleMaskImagePosition()
      }],
      "mask-image-b-to-pos": [{
        "mask-b-to": scaleMaskImagePosition()
      }],
      "mask-image-b-from-color": [{
        "mask-b-from": scaleColor()
      }],
      "mask-image-b-to-color": [{
        "mask-b-to": scaleColor()
      }],
      "mask-image-l-from-pos": [{
        "mask-l-from": scaleMaskImagePosition()
      }],
      "mask-image-l-to-pos": [{
        "mask-l-to": scaleMaskImagePosition()
      }],
      "mask-image-l-from-color": [{
        "mask-l-from": scaleColor()
      }],
      "mask-image-l-to-color": [{
        "mask-l-to": scaleColor()
      }],
      "mask-image-x-from-pos": [{
        "mask-x-from": scaleMaskImagePosition()
      }],
      "mask-image-x-to-pos": [{
        "mask-x-to": scaleMaskImagePosition()
      }],
      "mask-image-x-from-color": [{
        "mask-x-from": scaleColor()
      }],
      "mask-image-x-to-color": [{
        "mask-x-to": scaleColor()
      }],
      "mask-image-y-from-pos": [{
        "mask-y-from": scaleMaskImagePosition()
      }],
      "mask-image-y-to-pos": [{
        "mask-y-to": scaleMaskImagePosition()
      }],
      "mask-image-y-from-color": [{
        "mask-y-from": scaleColor()
      }],
      "mask-image-y-to-color": [{
        "mask-y-to": scaleColor()
      }],
      "mask-image-radial": [{
        "mask-radial": [isArbitraryVariable$1, isArbitraryValue$1]
      }],
      "mask-image-radial-from-pos": [{
        "mask-radial-from": scaleMaskImagePosition()
      }],
      "mask-image-radial-to-pos": [{
        "mask-radial-to": scaleMaskImagePosition()
      }],
      "mask-image-radial-from-color": [{
        "mask-radial-from": scaleColor()
      }],
      "mask-image-radial-to-color": [{
        "mask-radial-to": scaleColor()
      }],
      "mask-image-radial-shape": [{
        "mask-radial": ["circle", "ellipse"]
      }],
      "mask-image-radial-size": [{
        "mask-radial": [{
          closest: ["side", "corner"],
          farthest: ["side", "corner"]
        }]
      }],
      "mask-image-radial-pos": [{
        "mask-radial-at": scalePosition()
      }],
      "mask-image-conic-pos": [{
        "mask-conic": [isNumber$1]
      }],
      "mask-image-conic-from-pos": [{
        "mask-conic-from": scaleMaskImagePosition()
      }],
      "mask-image-conic-to-pos": [{
        "mask-conic-to": scaleMaskImagePosition()
      }],
      "mask-image-conic-from-color": [{
        "mask-conic-from": scaleColor()
      }],
      "mask-image-conic-to-color": [{
        "mask-conic-to": scaleColor()
      }],
      /**
       * Mask Mode
       * @see https://tailwindcss.com/docs/mask-mode
       */
      "mask-mode": [{
        mask: ["alpha", "luminance", "match"]
      }],
      /**
       * Mask Origin
       * @see https://tailwindcss.com/docs/mask-origin
       */
      "mask-origin": [{
        "mask-origin": ["border", "padding", "content", "fill", "stroke", "view"]
      }],
      /**
       * Mask Position
       * @see https://tailwindcss.com/docs/mask-position
       */
      "mask-position": [{
        mask: scaleBgPosition()
      }],
      /**
       * Mask Repeat
       * @see https://tailwindcss.com/docs/mask-repeat
       */
      "mask-repeat": [{
        mask: scaleBgRepeat()
      }],
      /**
       * Mask Size
       * @see https://tailwindcss.com/docs/mask-size
       */
      "mask-size": [{
        mask: scaleBgSize()
      }],
      /**
       * Mask Type
       * @see https://tailwindcss.com/docs/mask-type
       */
      "mask-type": [{
        "mask-type": ["alpha", "luminance"]
      }],
      /**
       * Mask Image
       * @see https://tailwindcss.com/docs/mask-image
       */
      "mask-image": [{
        mask: ["none", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      // ---------------
      // --- Filters ---
      // ---------------
      /**
       * Filter
       * @see https://tailwindcss.com/docs/filter
       */
      filter: [{
        filter: [
          // Deprecated since Tailwind CSS v3.0.0
          "",
          "none",
          isArbitraryVariable$1,
          isArbitraryValue$1
        ]
      }],
      /**
       * Blur
       * @see https://tailwindcss.com/docs/blur
       */
      blur: [{
        blur: scaleBlur()
      }],
      /**
       * Brightness
       * @see https://tailwindcss.com/docs/brightness
       */
      brightness: [{
        brightness: [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Contrast
       * @see https://tailwindcss.com/docs/contrast
       */
      contrast: [{
        contrast: [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Drop Shadow
       * @see https://tailwindcss.com/docs/drop-shadow
       */
      "drop-shadow": [{
        "drop-shadow": [
          // Deprecated since Tailwind CSS v4.0.0
          "",
          "none",
          themeDropShadow,
          isArbitraryVariableShadow$1,
          isArbitraryShadow$1
        ]
      }],
      /**
       * Drop Shadow Color
       * @see https://tailwindcss.com/docs/filter-drop-shadow#setting-the-shadow-color
       */
      "drop-shadow-color": [{
        "drop-shadow": scaleColor()
      }],
      /**
       * Grayscale
       * @see https://tailwindcss.com/docs/grayscale
       */
      grayscale: [{
        grayscale: ["", isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Hue Rotate
       * @see https://tailwindcss.com/docs/hue-rotate
       */
      "hue-rotate": [{
        "hue-rotate": [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Invert
       * @see https://tailwindcss.com/docs/invert
       */
      invert: [{
        invert: ["", isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Saturate
       * @see https://tailwindcss.com/docs/saturate
       */
      saturate: [{
        saturate: [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Sepia
       * @see https://tailwindcss.com/docs/sepia
       */
      sepia: [{
        sepia: ["", isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Backdrop Filter
       * @see https://tailwindcss.com/docs/backdrop-filter
       */
      "backdrop-filter": [{
        "backdrop-filter": [
          // Deprecated since Tailwind CSS v3.0.0
          "",
          "none",
          isArbitraryVariable$1,
          isArbitraryValue$1
        ]
      }],
      /**
       * Backdrop Blur
       * @see https://tailwindcss.com/docs/backdrop-blur
       */
      "backdrop-blur": [{
        "backdrop-blur": scaleBlur()
      }],
      /**
       * Backdrop Brightness
       * @see https://tailwindcss.com/docs/backdrop-brightness
       */
      "backdrop-brightness": [{
        "backdrop-brightness": [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Backdrop Contrast
       * @see https://tailwindcss.com/docs/backdrop-contrast
       */
      "backdrop-contrast": [{
        "backdrop-contrast": [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Backdrop Grayscale
       * @see https://tailwindcss.com/docs/backdrop-grayscale
       */
      "backdrop-grayscale": [{
        "backdrop-grayscale": ["", isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Backdrop Hue Rotate
       * @see https://tailwindcss.com/docs/backdrop-hue-rotate
       */
      "backdrop-hue-rotate": [{
        "backdrop-hue-rotate": [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Backdrop Invert
       * @see https://tailwindcss.com/docs/backdrop-invert
       */
      "backdrop-invert": [{
        "backdrop-invert": ["", isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Backdrop Opacity
       * @see https://tailwindcss.com/docs/backdrop-opacity
       */
      "backdrop-opacity": [{
        "backdrop-opacity": [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Backdrop Saturate
       * @see https://tailwindcss.com/docs/backdrop-saturate
       */
      "backdrop-saturate": [{
        "backdrop-saturate": [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Backdrop Sepia
       * @see https://tailwindcss.com/docs/backdrop-sepia
       */
      "backdrop-sepia": [{
        "backdrop-sepia": ["", isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      // --------------
      // --- Tables ---
      // --------------
      /**
       * Border Collapse
       * @see https://tailwindcss.com/docs/border-collapse
       */
      "border-collapse": [{
        border: ["collapse", "separate"]
      }],
      /**
       * Border Spacing
       * @see https://tailwindcss.com/docs/border-spacing
       */
      "border-spacing": [{
        "border-spacing": scaleUnambiguousSpacing()
      }],
      /**
       * Border Spacing X
       * @see https://tailwindcss.com/docs/border-spacing
       */
      "border-spacing-x": [{
        "border-spacing-x": scaleUnambiguousSpacing()
      }],
      /**
       * Border Spacing Y
       * @see https://tailwindcss.com/docs/border-spacing
       */
      "border-spacing-y": [{
        "border-spacing-y": scaleUnambiguousSpacing()
      }],
      /**
       * Table Layout
       * @see https://tailwindcss.com/docs/table-layout
       */
      "table-layout": [{
        table: ["auto", "fixed"]
      }],
      /**
       * Caption Side
       * @see https://tailwindcss.com/docs/caption-side
       */
      caption: [{
        caption: ["top", "bottom"]
      }],
      // ---------------------------------
      // --- Transitions and Animation ---
      // ---------------------------------
      /**
       * Transition Property
       * @see https://tailwindcss.com/docs/transition-property
       */
      transition: [{
        transition: ["", "all", "colors", "opacity", "shadow", "transform", "none", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Transition Behavior
       * @see https://tailwindcss.com/docs/transition-behavior
       */
      "transition-behavior": [{
        transition: ["normal", "discrete"]
      }],
      /**
       * Transition Duration
       * @see https://tailwindcss.com/docs/transition-duration
       */
      duration: [{
        duration: [isNumber$1, "initial", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Transition Timing Function
       * @see https://tailwindcss.com/docs/transition-timing-function
       */
      ease: [{
        ease: ["linear", "initial", themeEase, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Transition Delay
       * @see https://tailwindcss.com/docs/transition-delay
       */
      delay: [{
        delay: [isNumber$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Animation
       * @see https://tailwindcss.com/docs/animation
       */
      animate: [{
        animate: ["none", themeAnimate, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      // ------------------
      // --- Transforms ---
      // ------------------
      /**
       * Backface Visibility
       * @see https://tailwindcss.com/docs/backface-visibility
       */
      backface: [{
        backface: ["hidden", "visible"]
      }],
      /**
       * Perspective
       * @see https://tailwindcss.com/docs/perspective
       */
      perspective: [{
        perspective: [themePerspective, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Perspective Origin
       * @see https://tailwindcss.com/docs/perspective-origin
       */
      "perspective-origin": [{
        "perspective-origin": scalePositionWithArbitrary()
      }],
      /**
       * Rotate
       * @see https://tailwindcss.com/docs/rotate
       */
      rotate: [{
        rotate: scaleRotate()
      }],
      /**
       * Rotate X
       * @see https://tailwindcss.com/docs/rotate
       */
      "rotate-x": [{
        "rotate-x": scaleRotate()
      }],
      /**
       * Rotate Y
       * @see https://tailwindcss.com/docs/rotate
       */
      "rotate-y": [{
        "rotate-y": scaleRotate()
      }],
      /**
       * Rotate Z
       * @see https://tailwindcss.com/docs/rotate
       */
      "rotate-z": [{
        "rotate-z": scaleRotate()
      }],
      /**
       * Scale
       * @see https://tailwindcss.com/docs/scale
       */
      scale: [{
        scale: scaleScale()
      }],
      /**
       * Scale X
       * @see https://tailwindcss.com/docs/scale
       */
      "scale-x": [{
        "scale-x": scaleScale()
      }],
      /**
       * Scale Y
       * @see https://tailwindcss.com/docs/scale
       */
      "scale-y": [{
        "scale-y": scaleScale()
      }],
      /**
       * Scale Z
       * @see https://tailwindcss.com/docs/scale
       */
      "scale-z": [{
        "scale-z": scaleScale()
      }],
      /**
       * Scale 3D
       * @see https://tailwindcss.com/docs/scale
       */
      "scale-3d": ["scale-3d"],
      /**
       * Skew
       * @see https://tailwindcss.com/docs/skew
       */
      skew: [{
        skew: scaleSkew()
      }],
      /**
       * Skew X
       * @see https://tailwindcss.com/docs/skew
       */
      "skew-x": [{
        "skew-x": scaleSkew()
      }],
      /**
       * Skew Y
       * @see https://tailwindcss.com/docs/skew
       */
      "skew-y": [{
        "skew-y": scaleSkew()
      }],
      /**
       * Transform
       * @see https://tailwindcss.com/docs/transform
       */
      transform: [{
        transform: [isArbitraryVariable$1, isArbitraryValue$1, "", "none", "gpu", "cpu"]
      }],
      /**
       * Transform Origin
       * @see https://tailwindcss.com/docs/transform-origin
       */
      "transform-origin": [{
        origin: scalePositionWithArbitrary()
      }],
      /**
       * Transform Style
       * @see https://tailwindcss.com/docs/transform-style
       */
      "transform-style": [{
        transform: ["3d", "flat"]
      }],
      /**
       * Translate
       * @see https://tailwindcss.com/docs/translate
       */
      translate: [{
        translate: scaleTranslate()
      }],
      /**
       * Translate X
       * @see https://tailwindcss.com/docs/translate
       */
      "translate-x": [{
        "translate-x": scaleTranslate()
      }],
      /**
       * Translate Y
       * @see https://tailwindcss.com/docs/translate
       */
      "translate-y": [{
        "translate-y": scaleTranslate()
      }],
      /**
       * Translate Z
       * @see https://tailwindcss.com/docs/translate
       */
      "translate-z": [{
        "translate-z": scaleTranslate()
      }],
      /**
       * Translate None
       * @see https://tailwindcss.com/docs/translate
       */
      "translate-none": ["translate-none"],
      /**
       * Zoom
       * @see https://tailwindcss.com/docs/zoom
       */
      zoom: [{
        zoom: [isInteger$1, isArbitraryVariable$1, isArbitraryValue$1]
      }],
      // ---------------------
      // --- Interactivity ---
      // ---------------------
      /**
       * Accent Color
       * @see https://tailwindcss.com/docs/accent-color
       */
      accent: [{
        accent: scaleColor()
      }],
      /**
       * Appearance
       * @see https://tailwindcss.com/docs/appearance
       */
      appearance: [{
        appearance: ["none", "auto"]
      }],
      /**
       * Caret Color
       * @see https://tailwindcss.com/docs/just-in-time-mode#caret-color-utilities
       */
      "caret-color": [{
        caret: scaleColor()
      }],
      /**
       * Color Scheme
       * @see https://tailwindcss.com/docs/color-scheme
       */
      "color-scheme": [{
        scheme: ["normal", "dark", "light", "light-dark", "only-dark", "only-light"]
      }],
      /**
       * Cursor
       * @see https://tailwindcss.com/docs/cursor
       */
      cursor: [{
        cursor: ["auto", "default", "pointer", "wait", "text", "move", "help", "not-allowed", "none", "context-menu", "progress", "cell", "crosshair", "vertical-text", "alias", "copy", "no-drop", "grab", "grabbing", "all-scroll", "col-resize", "row-resize", "n-resize", "e-resize", "s-resize", "w-resize", "ne-resize", "nw-resize", "se-resize", "sw-resize", "ew-resize", "ns-resize", "nesw-resize", "nwse-resize", "zoom-in", "zoom-out", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      /**
       * Field Sizing
       * @see https://tailwindcss.com/docs/field-sizing
       */
      "field-sizing": [{
        "field-sizing": ["fixed", "content"]
      }],
      /**
       * Pointer Events
       * @see https://tailwindcss.com/docs/pointer-events
       */
      "pointer-events": [{
        "pointer-events": ["auto", "none"]
      }],
      /**
       * Resize
       * @see https://tailwindcss.com/docs/resize
       */
      resize: [{
        resize: ["none", "", "y", "x"]
      }],
      /**
       * Scroll Behavior
       * @see https://tailwindcss.com/docs/scroll-behavior
       */
      "scroll-behavior": [{
        scroll: ["auto", "smooth"]
      }],
      /**
       * Scrollbar Thumb Color
       * @see https://tailwindcss.com/docs/scrollbar-color
       */
      "scrollbar-thumb-color": [{
        "scrollbar-thumb": scaleColor()
      }],
      /**
       * Scrollbar Track Color
       * @see https://tailwindcss.com/docs/scrollbar-color
       */
      "scrollbar-track-color": [{
        "scrollbar-track": scaleColor()
      }],
      /**
       * Scrollbar Gutter
       * @see https://tailwindcss.com/docs/scrollbar-gutter
       */
      "scrollbar-gutter": [{
        "scrollbar-gutter": ["auto", "stable", "both"]
      }],
      /**
       * Scrollbar Width
       * @see https://tailwindcss.com/docs/scrollbar-width
       */
      "scrollbar-w": [{
        scrollbar: ["auto", "thin", "none"]
      }],
      /**
       * Scroll Margin
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-m": [{
        "scroll-m": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Margin Inline
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mx": [{
        "scroll-mx": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Margin Block
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-my": [{
        "scroll-my": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Margin Inline Start
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-ms": [{
        "scroll-ms": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Margin Inline End
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-me": [{
        "scroll-me": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Margin Block Start
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mbs": [{
        "scroll-mbs": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Margin Block End
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mbe": [{
        "scroll-mbe": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Margin Top
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mt": [{
        "scroll-mt": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Margin Right
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mr": [{
        "scroll-mr": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Margin Bottom
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mb": [{
        "scroll-mb": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Margin Left
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-ml": [{
        "scroll-ml": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-p": [{
        "scroll-p": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding Inline
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-px": [{
        "scroll-px": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding Block
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-py": [{
        "scroll-py": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding Inline Start
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-ps": [{
        "scroll-ps": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding Inline End
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pe": [{
        "scroll-pe": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding Block Start
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pbs": [{
        "scroll-pbs": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding Block End
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pbe": [{
        "scroll-pbe": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding Top
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pt": [{
        "scroll-pt": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding Right
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pr": [{
        "scroll-pr": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding Bottom
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pb": [{
        "scroll-pb": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Padding Left
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pl": [{
        "scroll-pl": scaleUnambiguousSpacing()
      }],
      /**
       * Scroll Snap Align
       * @see https://tailwindcss.com/docs/scroll-snap-align
       */
      "snap-align": [{
        snap: ["start", "end", "center", "align-none"]
      }],
      /**
       * Scroll Snap Stop
       * @see https://tailwindcss.com/docs/scroll-snap-stop
       */
      "snap-stop": [{
        snap: ["normal", "always"]
      }],
      /**
       * Scroll Snap Type
       * @see https://tailwindcss.com/docs/scroll-snap-type
       */
      "snap-type": [{
        snap: ["none", "x", "y", "both"]
      }],
      /**
       * Scroll Snap Type Strictness
       * @see https://tailwindcss.com/docs/scroll-snap-type
       */
      "snap-strictness": [{
        snap: ["mandatory", "proximity"]
      }],
      /**
       * Touch Action
       * @see https://tailwindcss.com/docs/touch-action
       */
      touch: [{
        touch: ["auto", "none", "manipulation"]
      }],
      /**
       * Touch Action X
       * @see https://tailwindcss.com/docs/touch-action
       */
      "touch-x": [{
        "touch-pan": ["x", "left", "right"]
      }],
      /**
       * Touch Action Y
       * @see https://tailwindcss.com/docs/touch-action
       */
      "touch-y": [{
        "touch-pan": ["y", "up", "down"]
      }],
      /**
       * Touch Action Pinch Zoom
       * @see https://tailwindcss.com/docs/touch-action
       */
      "touch-pz": ["touch-pinch-zoom"],
      /**
       * User Select
       * @see https://tailwindcss.com/docs/user-select
       */
      select: [{
        select: ["none", "text", "all", "auto"]
      }],
      /**
       * Will Change
       * @see https://tailwindcss.com/docs/will-change
       */
      "will-change": [{
        "will-change": ["auto", "scroll", "contents", "transform", isArbitraryVariable$1, isArbitraryValue$1]
      }],
      // -----------
      // --- SVG ---
      // -----------
      /**
       * Fill
       * @see https://tailwindcss.com/docs/fill
       */
      fill: [{
        fill: ["none", ...scaleColor()]
      }],
      /**
       * Stroke Width
       * @see https://tailwindcss.com/docs/stroke-width
       */
      "stroke-w": [{
        stroke: [isNumber$1, isArbitraryVariableLength$1, isArbitraryLength$1, isArbitraryNumber$1]
      }],
      /**
       * Stroke
       * @see https://tailwindcss.com/docs/stroke
       */
      stroke: [{
        stroke: ["none", ...scaleColor()]
      }],
      // ---------------------
      // --- Accessibility ---
      // ---------------------
      /**
       * Forced Color Adjust
       * @see https://tailwindcss.com/docs/forced-color-adjust
       */
      "forced-color-adjust": [{
        "forced-color-adjust": ["auto", "none"]
      }]
    },
    conflictingClassGroups: {
      "container-named": ["container-type"],
      overflow: ["overflow-x", "overflow-y"],
      overscroll: ["overscroll-x", "overscroll-y"],
      inset: ["inset-x", "inset-y", "inset-bs", "inset-be", "start", "end", "top", "right", "bottom", "left"],
      "inset-x": ["start", "end", "right", "left"],
      "inset-y": ["inset-bs", "inset-be", "top", "bottom"],
      flex: ["basis", "grow", "shrink"],
      gap: ["gap-x", "gap-y"],
      p: ["px", "py", "ps", "pe", "pbs", "pbe", "pt", "pr", "pb", "pl"],
      px: ["ps", "pe", "pr", "pl"],
      py: ["pbs", "pbe", "pt", "pb"],
      m: ["mx", "my", "ms", "me", "mbs", "mbe", "mt", "mr", "mb", "ml"],
      mx: ["ms", "me", "mr", "ml"],
      my: ["mbs", "mbe", "mt", "mb"],
      size: ["w", "h"],
      "font-size": ["leading"],
      "fvn-normal": ["fvn-ordinal", "fvn-slashed-zero", "fvn-figure", "fvn-spacing", "fvn-fraction"],
      "fvn-ordinal": ["fvn-normal"],
      "fvn-slashed-zero": ["fvn-normal"],
      "fvn-figure": ["fvn-normal"],
      "fvn-spacing": ["fvn-normal"],
      "fvn-fraction": ["fvn-normal"],
      "line-clamp": ["display", "overflow"],
      rounded: ["rounded-s", "rounded-e", "rounded-t", "rounded-r", "rounded-b", "rounded-l", "rounded-ss", "rounded-se", "rounded-ee", "rounded-es", "rounded-tl", "rounded-tr", "rounded-br", "rounded-bl"],
      "rounded-s": ["rounded-ss", "rounded-es"],
      "rounded-e": ["rounded-se", "rounded-ee"],
      "rounded-t": ["rounded-tl", "rounded-tr"],
      "rounded-r": ["rounded-tr", "rounded-br"],
      "rounded-b": ["rounded-br", "rounded-bl"],
      "rounded-l": ["rounded-tl", "rounded-bl"],
      "border-spacing": ["border-spacing-x", "border-spacing-y"],
      "border-w": ["border-w-x", "border-w-y", "border-w-s", "border-w-e", "border-w-bs", "border-w-be", "border-w-t", "border-w-r", "border-w-b", "border-w-l"],
      "border-w-x": ["border-w-s", "border-w-e", "border-w-r", "border-w-l"],
      "border-w-y": ["border-w-bs", "border-w-be", "border-w-t", "border-w-b"],
      "border-color": ["border-color-x", "border-color-y", "border-color-s", "border-color-e", "border-color-bs", "border-color-be", "border-color-t", "border-color-r", "border-color-b", "border-color-l"],
      "border-color-x": ["border-color-s", "border-color-e", "border-color-r", "border-color-l"],
      "border-color-y": ["border-color-bs", "border-color-be", "border-color-t", "border-color-b"],
      translate: ["translate-x", "translate-y", "translate-none"],
      "translate-none": ["translate", "translate-x", "translate-y", "translate-z"],
      "scroll-m": ["scroll-mx", "scroll-my", "scroll-ms", "scroll-me", "scroll-mbs", "scroll-mbe", "scroll-mt", "scroll-mr", "scroll-mb", "scroll-ml"],
      "scroll-mx": ["scroll-ms", "scroll-me", "scroll-mr", "scroll-ml"],
      "scroll-my": ["scroll-mbs", "scroll-mbe", "scroll-mt", "scroll-mb"],
      "scroll-p": ["scroll-px", "scroll-py", "scroll-ps", "scroll-pe", "scroll-pbs", "scroll-pbe", "scroll-pt", "scroll-pr", "scroll-pb", "scroll-pl"],
      "scroll-px": ["scroll-ps", "scroll-pe", "scroll-pr", "scroll-pl"],
      "scroll-py": ["scroll-pbs", "scroll-pbe", "scroll-pt", "scroll-pb"],
      touch: ["touch-x", "touch-y", "touch-pz"],
      "touch-x": ["touch"],
      "touch-y": ["touch"],
      "touch-pz": ["touch"]
    },
    conflictingClassGroupModifiers: {
      "font-size": ["leading"]
    },
    postfixLookupClassGroups: ["container-type"],
    orderSensitiveModifiers: ["*", "**", "after", "backdrop", "before", "details-content", "file", "first-letter", "first-line", "marker", "placeholder", "selection"]
  };
};
const twMerge = /* @__PURE__ */ createTailwindMerge$1(getDefaultConfig$1);
function cn(...inputs) {
  return twMerge(clsx$1(...inputs));
}
var isArray = Array.isArray;
var joinClassValue = (value) => {
  if (!value && value !== 0 && value !== 0n) return "";
  if (typeof value === "string") return value;
  if (typeof value === "number") {
    if (value !== value) return "";
    return "" + value;
  }
  if (typeof value === "bigint") return "" + value;
  let result = "";
  if (isArray(value)) {
    const length = value.length;
    for (let index = 0; index < length; index++) {
      const item = value[index];
      if (!item && item !== 0 && item !== 0n) continue;
      const resolved = typeof item === "string" ? item : joinClassValue(item);
      if (resolved) {
        if (result) result += " ";
        result += resolved;
      }
    }
    return result;
  }
  if (typeof value === "object") {
    for (const key in value) {
      if (value[key]) {
        if (result) result += " ";
        result += key;
      }
    }
  }
  return result;
};
var SPACE_REGEX = /\s+/g;
var isArray2 = Array.isArray;
var removeExtraSpaces = (str) => {
  if (typeof str !== "string" || !str) return str;
  return str.replace(SPACE_REGEX, " ").trim();
};
var stringNeedsNormalize = (str) => {
  const len = str.length;
  if (len === 0) return false;
  const first = str.charCodeAt(0);
  const last = str.charCodeAt(len - 1);
  if (first === 32 || last === 32 || first >= 9 && first <= 13 || first === 160 || last >= 9 && last <= 13 || last === 160) {
    return true;
  }
  for (let i = 0; i < len; i++) {
    const code = str.charCodeAt(i);
    if (code >= 9 && code <= 13 || code === 160) return true;
    if (code === 32 && i + 1 < len && str.charCodeAt(i + 1) === 32) return true;
  }
  return false;
};
var cx = (...classnames) => {
  const result = joinClassValue(classnames);
  if (!result) return void 0;
  return stringNeedsNormalize(result) ? removeExtraSpaces(result) : result;
};
var falsyToString = (value) => value === false ? "false" : value === true ? "true" : value === 0 ? "0" : value;
var isEmptyObject = (obj) => {
  if (!obj || typeof obj !== "object") return true;
  for (const _ in obj) return false;
  return true;
};
var isEqual = (obj1, obj2) => {
  if (obj1 === obj2) return true;
  if (!obj1 || !obj2) return false;
  const record1 = obj1;
  const record2 = obj2;
  const keys1 = Object.keys(record1);
  const keys2 = Object.keys(record2);
  if (keys1.length !== keys2.length) return false;
  for (let i = 0; i < keys1.length; i++) {
    const key = keys1[i];
    if (!keys2.includes(key)) return false;
    if (record1[key] !== record2[key]) return false;
  }
  return true;
};
var joinObjects = (obj1, obj2) => {
  const target = obj1;
  for (const key in obj2) {
    if (Object.hasOwn(obj2, key)) {
      const val2 = obj2[key];
      if (key in target) {
        target[key] = cx(target[key], val2);
      } else {
        target[key] = val2;
      }
    }
  }
  return obj1;
};
var flat = (arr, target) => {
  for (let i = 0; i < arr.length; i++) {
    const el = arr[i];
    if (isArray2(el)) flat(el, target);
    else if (el) target.push(el);
  }
};
var flatMergeArrays = (...arrays) => {
  const result = [];
  flat(arrays, result);
  const filtered = [];
  for (let i = 0; i < result.length; i++) {
    if (result[i]) filtered.push(result[i]);
  }
  return filtered;
};
var mergeObjects = (obj1, obj2) => {
  const record1 = obj1;
  const record2 = obj2;
  const result = {};
  for (const key in record1) {
    const val1 = record1[key];
    if (key in record2) {
      const val2 = record2[key];
      if (isArray2(val1) || isArray2(val2)) {
        result[key] = flatMergeArrays(val2, val1);
      } else if (typeof val1 === "object" && typeof val2 === "object" && val1 && val2) {
        result[key] = mergeObjects(val1, val2);
      } else {
        result[key] = val2 + " " + val1;
      }
    } else {
      result[key] = val1;
    }
  }
  for (const key in record2) {
    if (!(key in record1)) {
      result[key] = record2[key];
    }
  }
  return result;
};
var defaultConfig = {
  twMerge: true,
  twMergeConfig: {}
};
var VARIANT_CACHE_LIMIT = 256;
var OVERRIDE_CACHE_LIMIT = 128;
var CACHE_MISS = /* @__PURE__ */ Symbol("tv-cache-miss");
var hasClassOverride = (props) => (props == null ? void 0 : props.class) != null && props.class !== "" || (props == null ? void 0 : props.className) != null && props.className !== "";
var serializeFingerprintValue = (value) => {
  if (value === void 0) return "";
  if (value === null) return "null";
  if (typeof value === "string") return value;
  if (typeof value === "boolean") return value ? "true" : "false";
  if (typeof value === "number") return value === 0 ? "0" : String(value);
  if (typeof value === "bigint") return String(value);
  const mapped = falsyToString(value);
  const mappedType = typeof mapped;
  if (mappedType === "string" || mappedType === "number" || mappedType === "boolean" || mappedType === "bigint") {
    return String(mapped);
  }
  if (mappedType === "object") {
    try {
      return JSON.stringify(mapped);
    } catch {
      return null;
    }
  }
  return null;
};
var appendSignatureValue = (out, value) => {
  if (value === void 0) return out;
  if (value === null) return out + "null";
  const type = typeof value;
  if (type === "string" || type === "number" || type === "boolean" || type === "bigint") {
    return out + String(value);
  }
  if (Array.isArray(value)) {
    return out + value.join("\0");
  }
  try {
    return out + JSON.stringify(value);
  } catch {
    return out + "?";
  }
};
var buildPropsFingerprint = (variantKeys, defaultVariants, props, slotProps) => {
  let fingerprint = "";
  const seen = /* @__PURE__ */ Object.create(null);
  for (let i = 0; i < variantKeys.length; i++) {
    const key = variantKeys[i];
    seen[key] = 1;
    let value = defaultVariants[key];
    if (props && props[key] !== void 0) value = props[key];
    const serialized = serializeFingerprintValue(value);
    if (serialized === null) return null;
    fingerprint += key + ":" + serialized + ";";
  }
  const extras = [];
  for (const key in defaultVariants) {
    if (key === "class" || key === "className" || seen[key]) continue;
    seen[key] = 1;
    extras.push(key);
  }
  if (props) {
    for (const key in props) {
      if (key === "class" || key === "className" || seen[key] || props[key] === void 0) continue;
      seen[key] = 1;
      extras.push(key);
    }
  }
  if (extras.length > 1) extras.sort();
  for (let i = 0; i < extras.length; i++) {
    const key = extras[i];
    let value = defaultVariants[key];
    if (props && props[key] !== void 0) value = props[key];
    const serialized = serializeFingerprintValue(value);
    if (serialized === null) return null;
    fingerprint += key + ":" + serialized + ";";
  }
  return fingerprint;
};
var buildCompoundsSignature = (compoundVariants, compoundSlots) => {
  let signature = "";
  for (let i = 0; i < compoundVariants.length; i++) {
    const { conditionKeys, source } = compoundVariants[i];
    for (let j = 0; j < conditionKeys.length; j++) {
      const key = conditionKeys[j];
      signature += key + "=";
      signature = appendSignatureValue(signature, source[key]);
      signature += ",";
    }
    signature += "c=";
    signature = appendSignatureValue(signature, source.class);
    signature += "|cn=";
    signature = appendSignatureValue(signature, source.className);
    signature += ";";
  }
  for (let i = 0; i < compoundSlots.length; i++) {
    const { conditionKeys, source } = compoundSlots[i];
    for (let j = 0; j < conditionKeys.length; j++) {
      const key = conditionKeys[j];
      signature += key + "=";
      signature = appendSignatureValue(signature, source[key]);
      signature += ",";
    }
    if (Array.isArray(source.slots)) {
      signature += "slots=" + source.slots.join(",") + ",";
    }
    signature += "c=";
    signature = appendSignatureValue(signature, source.class);
    signature += "|cn=";
    signature = appendSignatureValue(signature, source.className);
    signature += ";";
  }
  return signature;
};
var createBoundedCache = (limit = VARIANT_CACHE_LIMIT) => {
  let primary = /* @__PURE__ */ new Map();
  let secondary = null;
  return {
    get(key) {
      if (primary.has(key)) return primary.get(key);
      if (secondary == null ? void 0 : secondary.has(key)) {
        const value = secondary.get(key);
        primary.set(key, value);
        return value;
      }
      return CACHE_MISS;
    },
    set(key, value) {
      if (primary.size >= limit) {
        secondary = primary;
        primary = /* @__PURE__ */ new Map();
      }
      primary.set(key, value);
    }
  };
};
var createResultCache = (limit = VARIANT_CACHE_LIMIT) => {
  const cache = createBoundedCache(limit);
  return {
    get(key) {
      return cache.get(key);
    },
    set(key, value) {
      cache.set(key, value);
    }
  };
};
var createNestedOverrideCache = (limit = OVERRIDE_CACHE_LIMIT) => {
  let primary = /* @__PURE__ */ new Map();
  let secondary = null;
  let size = 0;
  return {
    get(coreKey, overrideKey) {
      const primaryInner = primary.get(coreKey);
      if (primaryInner) {
        const value = primaryInner.get(overrideKey);
        if (value !== void 0 || primaryInner.has(overrideKey)) return value;
      }
      if (secondary) {
        const secondaryInner = secondary.get(coreKey);
        if (secondaryInner) {
          const value = secondaryInner.get(overrideKey);
          if (value !== void 0 || secondaryInner.has(overrideKey)) {
            let promoteInner = primary.get(coreKey);
            if (!promoteInner) {
              promoteInner = /* @__PURE__ */ new Map();
              primary.set(coreKey, promoteInner);
            }
            if (!promoteInner.has(overrideKey)) size++;
            promoteInner.set(overrideKey, value);
            return value;
          }
        }
      }
      return CACHE_MISS;
    },
    set(coreKey, overrideKey, value) {
      if (size >= limit) {
        secondary = primary;
        primary = /* @__PURE__ */ new Map();
        size = 0;
      }
      let inner = primary.get(coreKey);
      if (!inner) {
        inner = /* @__PURE__ */ new Map();
        primary.set(coreKey, inner);
      }
      if (!inner.has(overrideKey)) size++;
      inner.set(overrideKey, value);
    }
  };
};
var createLazyOverrideMerge = (cn2, config) => {
  let cache = null;
  return (core, props) => {
    if (!hasClassOverride(props)) return core;
    const classVal = props.class;
    const classNameVal = props.className;
    if (classVal != null && classVal !== "" && typeof classVal !== "string" || classNameVal != null && classNameVal !== "" && typeof classNameVal !== "string") {
      return cn2(config, core, classVal, classNameVal);
    }
    cache ?? (cache = createNestedOverrideCache());
    const coreKey = core ?? "";
    const overrideKey = (typeof classVal === "string" ? classVal : "") + "\0" + (typeof classNameVal === "string" ? classNameVal : "");
    const cached = cache.get(coreKey, overrideKey);
    if (cached !== CACHE_MISS) return cached;
    const merged = cn2(config, core, classVal, classNameVal);
    cache.set(coreKey, overrideKey, merged);
    return merged;
  };
};
function createState() {
  let cachedTwMerge = null;
  let cachedTwMergeConfig = {};
  let didTwMergeConfigChange = false;
  return {
    get cachedTwMerge() {
      return cachedTwMerge;
    },
    set cachedTwMerge(value) {
      cachedTwMerge = value;
    },
    get cachedTwMergeConfig() {
      return cachedTwMergeConfig;
    },
    set cachedTwMergeConfig(value) {
      cachedTwMergeConfig = value;
    },
    get didTwMergeConfigChange() {
      return didTwMergeConfigChange;
    },
    set didTwMergeConfigChange(value) {
      didTwMergeConfigChange = value;
    },
    reset() {
      cachedTwMerge = null;
      cachedTwMergeConfig = {};
      didTwMergeConfigChange = false;
    }
  };
}
var state = createState();
var synchronizeTwMergeConfig = (config) => {
  if (!isEmptyObject(config.twMergeConfig) && !isEqual(config.twMergeConfig, state.cachedTwMergeConfig)) {
    state.didTwMergeConfigChange = true;
    state.cachedTwMergeConfig = config.twMergeConfig;
  }
};
var compileVariants = (variants, variantKeys) => {
  const compiledVariants = [];
  for (let i = 0; i < variantKeys.length; i++) {
    const key = variantKeys[i];
    const values = variants[key];
    compiledVariants.push({ key, values, isEmpty: isEmptyObject(values) });
  }
  return compiledVariants;
};
var compileCompoundVariants = (compoundVariants) => {
  if (!Array.isArray(compoundVariants) || compoundVariants.length === 0) return [];
  const result = [];
  for (let i = 0; i < compoundVariants.length; i++) {
    const compoundVariant = compoundVariants[i];
    const conditionKeys = [];
    for (const key in compoundVariant) {
      if (key !== "class" && key !== "className") {
        conditionKeys.push(key);
      }
    }
    result.push({ conditionKeys, source: compoundVariant });
  }
  return result;
};
var compileCompoundSlots = (compoundSlots) => {
  if (!Array.isArray(compoundSlots) || compoundSlots.length === 0) return [];
  const result = [];
  for (let i = 0; i < compoundSlots.length; i++) {
    const compoundSlot = compoundSlots[i];
    const conditionKeys = [];
    for (const key in compoundSlot) {
      if (key !== "slots" && key !== "class" && key !== "className") {
        conditionKeys.push(key);
      }
    }
    result.push({ conditionKeys, source: compoundSlot });
  }
  return result;
};
var indexCompoundSlotsBySlot = (compiledCompoundSlots) => {
  const index = {};
  for (let i = 0; i < compiledCompoundSlots.length; i++) {
    const compoundSlot = compiledCompoundSlots[i];
    const slots = compoundSlot.source.slots;
    if (!Array.isArray(slots)) continue;
    for (let j = 0; j < slots.length; j++) {
      const slotKey = slots[j];
      if (!index[slotKey]) index[slotKey] = [];
      index[slotKey].push(compoundSlot);
    }
  }
  return index;
};
var resolveOptions = (options, configProp) => {
  const {
    extend = null,
    slots: slotProps = {},
    variants: variantsProps = {},
    compoundVariants: compoundVariantsProps = [],
    compoundSlots: compoundSlotsProps = [],
    defaultVariants: defaultVariantsProps = {}
  } = options;
  const config = { ...defaultConfig, ...configProp };
  const hasSlots = options.slots !== void 0;
  const base = (extend == null ? void 0 : extend.base) ? cx(extend.base, options == null ? void 0 : options.base) : options == null ? void 0 : options.base;
  const variants = (extend == null ? void 0 : extend.variants) && !isEmptyObject(extend.variants) ? mergeObjects(variantsProps, extend.variants) : variantsProps;
  const defaultVariants = (extend == null ? void 0 : extend.defaultVariants) && !isEmptyObject(extend.defaultVariants) ? { ...extend.defaultVariants, ...defaultVariantsProps } : defaultVariantsProps;
  synchronizeTwMergeConfig(config);
  const isExtendedSlotsEmpty = !(extend == null ? void 0 : extend.slots) || isEmptyObject(extend.slots);
  const componentBase = hasSlots ? isExtendedSlotsEmpty && (extend == null ? void 0 : extend.base) ? cx(options == null ? void 0 : options.base, extend.base) : typeof (options == null ? void 0 : options.base) === "string" || (options == null ? void 0 : options.base) == null ? options.base : cx(options.base) : void 0;
  const componentSlots = hasSlots ? {
    base: componentBase,
    ...slotProps
  } : {};
  const slots = isExtendedSlotsEmpty ? componentSlots : joinObjects(
    { ...extend == null ? void 0 : extend.slots },
    isEmptyObject(componentSlots) ? { base: options == null ? void 0 : options.base } : componentSlots
  );
  const compoundVariants = !(extend == null ? void 0 : extend.compoundVariants) || isEmptyObject(extend.compoundVariants) ? compoundVariantsProps : flatMergeArrays(extend == null ? void 0 : extend.compoundVariants, compoundVariantsProps);
  const compoundSlots = !(extend == null ? void 0 : extend.compoundSlots) || isEmptyObject(extend.compoundSlots) ? compoundSlotsProps : flatMergeArrays(extend == null ? void 0 : extend.compoundSlots, compoundSlotsProps);
  const variantKeys = Object.keys(variants);
  const deferredError = compoundVariants && !Array.isArray(compoundVariants) ? new TypeError(
    `The "compoundVariants" prop must be an array. Received: ${typeof compoundVariants}`
  ) : compoundSlots && !Array.isArray(compoundSlots) ? new TypeError(
    `The "compoundSlots" prop must be an array. Received: ${typeof compoundSlots}`
  ) : null;
  const mode = hasSlots || !isExtendedSlotsEmpty ? "slots" : variantKeys.length === 0 ? "plain" : "variants";
  return {
    config,
    extend,
    base,
    variants,
    defaultVariants,
    slots,
    compoundVariants,
    compoundSlots,
    compiledVariants: null,
    compiledCompoundVariants: null,
    compiledCompoundSlots: null,
    compiledCompoundSlotsBySlot: null,
    deferredError,
    mode,
    slotKeys: null,
    variantKeys
  };
};
var compileResolvedOptions = (resolved) => {
  if (resolved.compiledVariants !== null) return resolved;
  resolved.compiledVariants = compileVariants(resolved.variants, resolved.variantKeys);
  resolved.compiledCompoundVariants = compileCompoundVariants(resolved.compoundVariants);
  resolved.compiledCompoundSlots = compileCompoundSlots(resolved.compoundSlots);
  resolved.compiledCompoundSlotsBySlot = indexCompoundSlotsBySlot(resolved.compiledCompoundSlots);
  resolved.slotKeys = resolved.slots && typeof resolved.slots === "object" ? Object.keys(resolved.slots) : [];
  return resolved;
};
var EMPTY_ARRAY = [];
var variantClassesScratch = [];
var compoundClassesScratch = [];
var compoundVariantBySlotScratch = [];
var compoundSlotClassesScratch = [];
var getCompleteProps = (defaultVariants, props, slotProps) => {
  const result = {};
  for (const key in defaultVariants) {
    result[key] = defaultVariants[key];
  }
  if (props) {
    for (const key in props) {
      if (props[key] !== void 0) result[key] = props[key];
    }
  }
  if (slotProps) {
    for (const key in slotProps) {
      if (slotProps[key] !== void 0) result[key] = slotProps[key];
    }
  }
  return result;
};
var isNullishOrFalse = (value) => value == null || value === false;
var matchesCompoundValue = (expected, actual) => {
  if (!Array.isArray(expected)) {
    return expected === actual || isNullishOrFalse(expected) && isNullishOrFalse(actual);
  }
  for (let i = 0; i < expected.length; i++) {
    const expectedValue = expected[i];
    if (expectedValue === actual || isNullishOrFalse(expectedValue) && isNullishOrFalse(actual)) {
      return true;
    }
  }
  return false;
};
var getVariantValue = (variant, defaultVariants, props, slotProps) => {
  if (variant.isEmpty) return null;
  const variantProp = (slotProps == null ? void 0 : slotProps[variant.key]) ?? (props == null ? void 0 : props[variant.key]);
  if (variantProp === null) return null;
  const variantKey = falsyToString(variantProp);
  if (typeof variantKey === "object") return null;
  const defaultVariantProp = defaultVariants == null ? void 0 : defaultVariants[variant.key];
  const key = variantKey != null ? variantKey : falsyToString(defaultVariantProp);
  return variant.values[key || "false"];
};
var matchesConditions = (compound, completeProps) => {
  const { conditionKeys, source } = compound;
  for (let i = 0; i < conditionKeys.length; i++) {
    const key = conditionKeys[i];
    if (!matchesCompoundValue(source[key], completeProps[key])) return false;
  }
  return true;
};
var pushCompoundClassForSlot = (result, slotKey, classValue) => {
  if (typeof classValue === "string") {
    if (slotKey === "base") result.push(classValue);
  } else if (classValue && typeof classValue === "object" && classValue[slotKey]) {
    result.push(classValue[slotKey]);
  }
};
var getVariantClassNames = (variants, defaultVariants, props) => {
  const result = variantClassesScratch;
  result.length = 0;
  for (let i = 0; i < variants.length; i++) {
    const value = getVariantValue(variants[i], defaultVariants, props);
    if (value) result.push(value);
  }
  return result;
};
var getVariantClassNamesBySlot = (slotKey, variants, defaultVariants, props, slotProps) => {
  const result = variantClassesScratch;
  result.length = 0;
  for (let i = 0; i < variants.length; i++) {
    const variantValue = getVariantValue(variants[i], defaultVariants, props, slotProps);
    const value = slotKey === "base" && typeof variantValue === "string" ? variantValue : variantValue && variantValue[slotKey];
    if (value) result.push(value);
  }
  return result;
};
var getCompoundVariantClasses = (compoundVariants, completeProps) => {
  const result = compoundClassesScratch;
  result.length = 0;
  for (let i = 0; i < compoundVariants.length; i++) {
    const compoundVariant = compoundVariants[i];
    if (!matchesConditions(compoundVariant, completeProps)) continue;
    if (compoundVariant.source.class) result.push(compoundVariant.source.class);
    if (compoundVariant.source.className) result.push(compoundVariant.source.className);
  }
  return result;
};
var getCompoundVariantClassesBySlot = (slotKey, compoundVariants, completeProps) => {
  const result = compoundVariantBySlotScratch;
  result.length = 0;
  for (let i = 0; i < compoundVariants.length; i++) {
    const compoundVariant = compoundVariants[i];
    if (!matchesConditions(compoundVariant, completeProps)) continue;
    pushCompoundClassForSlot(result, slotKey, compoundVariant.source.class);
    pushCompoundClassForSlot(result, slotKey, compoundVariant.source.className);
  }
  return result;
};
var getCompoundSlotClasses = (compoundSlotsForKey, completeProps) => {
  const result = compoundSlotClassesScratch;
  result.length = 0;
  for (let i = 0; i < compoundSlotsForKey.length; i++) {
    const compoundSlot = compoundSlotsForKey[i];
    if (!matchesConditions(compoundSlot, completeProps)) continue;
    if (compoundSlot.source.class) result.push(compoundSlot.source.class);
    if (compoundSlot.source.className) result.push(compoundSlot.source.className);
  }
  return result;
};
var createPlainResolver = (resolved, cn2) => {
  const { base, config } = resolved;
  let core = CACHE_MISS;
  const mergeOverride = createLazyOverrideMerge(cn2, config);
  return ((props) => {
    if (core === CACHE_MISS) {
      core = cn2(config, base);
    }
    return mergeOverride(core, props);
  });
};
var createVariantResolver = (resolved, cn2) => {
  const { base, config, defaultVariants, deferredError, variantKeys } = resolved;
  let compiledCompoundVariants = resolved.compiledCompoundVariants;
  let compiledVariants = resolved.compiledVariants;
  let compiledCompoundSlots = EMPTY_ARRAY;
  let cache = null;
  const mergeOverride = createLazyOverrideMerge(cn2, config);
  let coldInvokesRemaining = 1;
  const computeCore = (props) => {
    const compoundClasses = compiledCompoundVariants.length > 0 ? getCompoundVariantClasses(
      compiledCompoundVariants,
      getCompleteProps(defaultVariants, props)
    ) : void 0;
    return cn2(
      config,
      base,
      getVariantClassNames(compiledVariants, defaultVariants, props),
      compoundClasses
    );
  };
  return ((props) => {
    if (deferredError) throw deferredError;
    if (compiledVariants === null || compiledCompoundVariants === null) {
      compileResolvedOptions(resolved);
      compiledVariants = resolved.compiledVariants;
      compiledCompoundVariants = resolved.compiledCompoundVariants;
      compiledCompoundSlots = resolved.compiledCompoundSlots ?? EMPTY_ARRAY;
    }
    let core;
    if (coldInvokesRemaining > 0) {
      coldInvokesRemaining--;
      core = computeCore(props);
    } else {
      cache ?? (cache = createResultCache());
      const propsFingerprint = buildPropsFingerprint(variantKeys, defaultVariants, props);
      if (propsFingerprint !== null) {
        const compoundsSig = compiledCompoundVariants.length > 0 || compiledCompoundSlots.length > 0 ? buildCompoundsSignature(compiledCompoundVariants, compiledCompoundSlots) : "";
        const cacheKey = propsFingerprint + "#" + compoundsSig;
        const cached = cache.get(cacheKey);
        if (cached !== CACHE_MISS) {
          core = cached;
        } else {
          core = computeCore(props);
          cache.set(cacheKey, core);
        }
      } else {
        core = computeCore(props);
      }
    }
    return mergeOverride(core, props);
  });
};
var createSlotsResolver = (resolved, cn2) => {
  const { config, defaultVariants, deferredError, slots, variantKeys } = resolved;
  let compoundVariants = null;
  let compoundSlots = null;
  let keys = null;
  let slotComputers = null;
  let hasCompounds = false;
  let mergeOverride = null;
  let parentCache = null;
  let coldParentInvokesRemaining = 1;
  const ensureCompiled = () => {
    if (keys !== null) return;
    if (resolved.compiledVariants === null || resolved.compiledCompoundVariants === null || resolved.compiledCompoundSlots === null || resolved.compiledCompoundSlotsBySlot === null || resolved.slotKeys === null) {
      compileResolvedOptions(resolved);
    }
    const variants = resolved.compiledVariants;
    compoundVariants = resolved.compiledCompoundVariants;
    compoundSlots = resolved.compiledCompoundSlots;
    const compoundSlotsBySlot = resolved.compiledCompoundSlotsBySlot;
    keys = resolved.slotKeys;
    hasCompounds = compoundVariants.length > 0 || compoundSlots.length > 0;
    mergeOverride = createLazyOverrideMerge(cn2, config);
    const computers = new Array(keys.length);
    for (let i = 0; i < keys.length; i++) {
      const slotKey = keys[i];
      const compoundSlotsForKey = compoundSlotsBySlot[slotKey] ?? EMPTY_ARRAY;
      computers[i] = (propsRef, slotProps) => {
        const completeProps = hasCompounds ? getCompleteProps(defaultVariants, propsRef, slotProps) : void 0;
        const compoundVariantClasses = completeProps ? getCompoundVariantClassesBySlot(slotKey, compoundVariants, completeProps) : void 0;
        const compoundSlotClasses = completeProps ? getCompoundSlotClasses(compoundSlotsForKey, completeProps) : void 0;
        return cn2(
          config,
          slots[slotKey],
          getVariantClassNamesBySlot(slotKey, variants, defaultVariants, propsRef, slotProps),
          compoundVariantClasses,
          compoundSlotClasses
        );
      };
    }
    slotComputers = computers;
  };
  const createSlotsResult = (props) => {
    const slotKeys = keys;
    const computers = slotComputers;
    const overrideMerge = mergeOverride;
    const result = {};
    for (let i = 0; i < slotKeys.length; i++) {
      const compute = computers[i];
      const core = compute(props);
      result[slotKeys[i]] = (slotProps) => {
        if (slotProps == null) return core;
        let hasVariantOverride = false;
        for (const key in slotProps) {
          if (key === "class" || key === "className") continue;
          if (slotProps[key] !== void 0) {
            hasVariantOverride = true;
            break;
          }
        }
        if (!hasVariantOverride) {
          return overrideMerge(core, slotProps);
        }
        return overrideMerge(compute(props, slotProps), slotProps);
      };
    }
    return result;
  };
  return ((props) => {
    if (deferredError) throw deferredError;
    ensureCompiled();
    if (coldParentInvokesRemaining > 0) {
      coldParentInvokesRemaining--;
      return createSlotsResult(props);
    }
    const propsFingerprint = buildPropsFingerprint(variantKeys, defaultVariants, props);
    if (propsFingerprint === null) {
      return createSlotsResult(props);
    }
    const compoundsSig = hasCompounds ? buildCompoundsSignature(compoundVariants, compoundSlots) : "";
    const cacheKey = propsFingerprint + "#" + compoundsSig;
    parentCache ?? (parentCache = createBoundedCache());
    const cached = parentCache.get(cacheKey);
    if (cached !== CACHE_MISS) return cached;
    const next = createSlotsResult(props);
    parentCache.set(cacheKey, next);
    return next;
  });
};
var createClassResolver = (resolved, cn2) => {
  if (resolved.mode === "plain") return createPlainResolver(resolved, cn2);
  let resolver;
  return ((props) => {
    resolver ?? (resolver = resolved.mode === "slots" ? createSlotsResolver(resolved, cn2) : createVariantResolver(resolved, cn2));
    return resolver(props);
  });
};
var attachComponentMetadata = (component, resolved) => {
  component.variantKeys = resolved.variantKeys;
  component.extend = resolved.extend;
  component.base = resolved.base;
  component.slots = resolved.slots;
  component.variants = resolved.variants;
  component.defaultVariants = resolved.defaultVariants;
  component.compoundSlots = resolved.compoundSlots;
  component.compoundVariants = resolved.compoundVariants;
};
var getTailwindVariants = (cn2) => {
  const tv2 = (options, configProp) => {
    const resolved = resolveOptions(options, configProp);
    const component = createClassResolver(resolved, cn2);
    attachComponentMetadata(component, resolved);
    return component;
  };
  const createTV = (configProp) => {
    return (options, config) => tv2(options, config ? mergeObjects(configProp, config) : configProp);
  };
  return {
    tv: tv2,
    createTV
  };
};
var concatArrays = (array1, array2) => {
  const length1 = array1.length;
  const length2 = array2.length;
  const combined = new Array(length1 + length2);
  for (let i = 0; i < length1; i++) combined[i] = array1[i];
  for (let i = 0; i < length2; i++) combined[length1 + i] = array2[i];
  return combined;
};
var createClassValidatorObject = (classGroupId, validator) => ({
  classGroupId,
  validator
});
var createClassPartObject = (nextPart = /* @__PURE__ */ new Map(), validators = null, classGroupId) => ({
  nextPart,
  validators,
  classGroupId
});
var CLASS_PART_SEPARATOR = "-";
var EMPTY_CONFLICTS = [];
var ARBITRARY_PROPERTY_PREFIX = "arbitrary..";
var createClassGroupUtils = (config) => {
  const classMap = createClassMap(config);
  const { conflictingClassGroups, conflictingClassGroupModifiers } = config;
  const getClassGroupId = (className) => {
    if (className[0] === "[" && className[className.length - 1] === "]") {
      return getGroupIdForArbitraryProperty(className);
    }
    const classParts = className.split(CLASS_PART_SEPARATOR);
    const startIndex = classParts[0] === "" && classParts.length > 1 ? 1 : 0;
    return getGroupRecursive(classParts, startIndex, classMap);
  };
  const getConflictingClassGroupIds = (classGroupId, hasPostfixModifier) => {
    if (hasPostfixModifier) {
      const modifierConflicts = conflictingClassGroupModifiers[classGroupId];
      const baseConflicts = conflictingClassGroups[classGroupId];
      if (modifierConflicts) {
        if (baseConflicts) {
          return concatArrays(baseConflicts, modifierConflicts);
        }
        return modifierConflicts;
      }
      return baseConflicts || EMPTY_CONFLICTS;
    }
    return conflictingClassGroups[classGroupId] || EMPTY_CONFLICTS;
  };
  return {
    getClassGroupId,
    getConflictingClassGroupIds
  };
};
var getGroupRecursive = (classParts, startIndex, classPartObject) => {
  const classPathsLength = classParts.length - startIndex;
  if (classPathsLength === 0) {
    return classPartObject.classGroupId;
  }
  const currentClassPart = classParts[startIndex];
  const nextClassPartObject = classPartObject.nextPart.get(currentClassPart);
  if (nextClassPartObject) {
    const result = getGroupRecursive(classParts, startIndex + 1, nextClassPartObject);
    if (result) return result;
  }
  const validators = classPartObject.validators;
  if (validators === null) {
    return void 0;
  }
  const classRest = startIndex === 0 ? classParts.join(CLASS_PART_SEPARATOR) : classParts.slice(startIndex).join(CLASS_PART_SEPARATOR);
  const validatorsLength = validators.length;
  for (let index = 0; index < validatorsLength; index++) {
    const validatorObject = validators[index];
    if (validatorObject.validator(classRest)) {
      return validatorObject.classGroupId;
    }
  }
  return void 0;
};
var getGroupIdForArbitraryProperty = (className) => {
  const content = className.slice(1, -1);
  const colonIndex = content.indexOf(":");
  if (colonIndex === -1) {
    return void 0;
  }
  const property = content.slice(0, colonIndex);
  return property ? ARBITRARY_PROPERTY_PREFIX + property : void 0;
};
var createClassMap = (config) => {
  const { theme, classGroups } = config;
  return processClassGroups(classGroups, theme);
};
var processClassGroups = (classGroups, theme) => {
  const classMap = createClassPartObject();
  for (const classGroupId in classGroups) {
    const group = classGroups[classGroupId];
    processClassesRecursively(group, classMap, classGroupId, theme);
  }
  return classMap;
};
var processClassesRecursively = (classGroup, classPartObject, classGroupId, theme) => {
  const length = classGroup.length;
  for (let index = 0; index < length; index++) {
    const classDefinition = classGroup[index];
    processClassDefinition(classDefinition, classPartObject, classGroupId, theme);
  }
};
var processClassDefinition = (classDefinition, classPartObject, classGroupId, theme) => {
  if (typeof classDefinition === "string") {
    processStringDefinition(classDefinition, classPartObject, classGroupId);
    return;
  }
  if (typeof classDefinition === "function") {
    processFunctionDefinition(classDefinition, classPartObject, classGroupId, theme);
    return;
  }
  processObjectDefinition(
    classDefinition,
    classPartObject,
    classGroupId,
    theme
  );
};
var processStringDefinition = (classDefinition, classPartObject, classGroupId) => {
  const classPartObjectToEdit = classDefinition === "" ? classPartObject : getPart(classPartObject, classDefinition);
  classPartObjectToEdit.classGroupId = classGroupId;
};
var processFunctionDefinition = (classDefinition, classPartObject, classGroupId, theme) => {
  if (isThemeGetter(classDefinition)) {
    processClassesRecursively(classDefinition(theme), classPartObject, classGroupId, theme);
    return;
  }
  if (classPartObject.validators === null) {
    classPartObject.validators = [];
  }
  classPartObject.validators.push(
    createClassValidatorObject(classGroupId, classDefinition)
  );
};
var processObjectDefinition = (classDefinition, classPartObject, classGroupId, theme) => {
  const entries = Object.entries(classDefinition);
  const length = entries.length;
  for (let index = 0; index < length; index++) {
    const [key, value] = entries[index];
    processClassesRecursively(value, getPart(classPartObject, key), classGroupId, theme);
  }
};
var getPart = (classPartObject, path) => {
  let current = classPartObject;
  const parts = path.split(CLASS_PART_SEPARATOR);
  const length = parts.length;
  for (let index = 0; index < length; index++) {
    const part = parts[index];
    let next = current.nextPart.get(part);
    if (!next) {
      next = createClassPartObject();
      current.nextPart.set(part, next);
    }
    current = next;
  }
  return current;
};
var isThemeGetter = (classDefinition) => "isThemeGetter" in classDefinition && classDefinition.isThemeGetter === true;
var IMPORTANT_MODIFIER = "!";
var CHAR_MODIFIER_SEPARATOR = 58;
var CHAR_POSTFIX_SEPARATOR = 47;
var CHAR_OPEN_BRACKET = 91;
var CHAR_CLOSE_BRACKET = 93;
var CHAR_OPEN_PAREN = 40;
var CHAR_CLOSE_PAREN = 41;
var CHAR_IMPORTANT = 33;
var createResultObject = (modifiers, hasImportantModifier, baseClassName, maybePostfixModifierPosition) => ({
  modifiers,
  hasImportantModifier,
  baseClassName,
  maybePostfixModifierPosition,
  isExternal: void 0
});
var parseClassName = (className) => {
  const modifiers = [];
  let bracketDepth = 0;
  let parenDepth = 0;
  let modifierStart = 0;
  let postfixModifierPosition;
  const len = className.length;
  for (let index = 0; index < len; index++) {
    const charCode = className.charCodeAt(index);
    if (bracketDepth === 0 && parenDepth === 0) {
      if (charCode === CHAR_MODIFIER_SEPARATOR) {
        modifiers.push(className.slice(modifierStart, index));
        modifierStart = index + 1;
        continue;
      }
      if (charCode === CHAR_POSTFIX_SEPARATOR) {
        postfixModifierPosition = index;
        continue;
      }
    }
    if (charCode === CHAR_OPEN_BRACKET) bracketDepth++;
    else if (charCode === CHAR_CLOSE_BRACKET) bracketDepth--;
    else if (charCode === CHAR_OPEN_PAREN) parenDepth++;
    else if (charCode === CHAR_CLOSE_PAREN) parenDepth--;
  }
  const baseClassNameWithImportantModifier = modifiers.length === 0 ? className : className.slice(modifierStart);
  let baseClassName = baseClassNameWithImportantModifier;
  let hasImportantModifier = false;
  const lastIndex = baseClassNameWithImportantModifier.length - 1;
  if (baseClassNameWithImportantModifier.charCodeAt(lastIndex) === CHAR_IMPORTANT) {
    baseClassName = baseClassNameWithImportantModifier.slice(0, -1);
    hasImportantModifier = true;
  } else if (
    // Tailwind CSS v3: important modifier at the start of the base class (legacy).
    baseClassNameWithImportantModifier.charCodeAt(0) === CHAR_IMPORTANT
  ) {
    baseClassName = baseClassNameWithImportantModifier.slice(1);
    hasImportantModifier = true;
  }
  const maybePostfixModifierPosition = postfixModifierPosition && postfixModifierPosition > modifierStart ? postfixModifierPosition - modifierStart : void 0;
  return createResultObject(
    modifiers,
    hasImportantModifier,
    baseClassName,
    maybePostfixModifierPosition
  );
};
var createSortModifiers = (config) => {
  const orderSensitiveModifiers = new Set(config.orderSensitiveModifiers);
  return (modifiers) => {
    const result = [];
    let currentSegment = [];
    for (let index = 0; index < modifiers.length; index++) {
      const modifier = modifiers[index];
      const isArbitrary = modifier[0] === "[";
      const isOrderSensitive = orderSensitiveModifiers.has(modifier);
      if (isArbitrary || isOrderSensitive) {
        if (currentSegment.length > 0) {
          currentSegment.sort();
          for (let segmentIndex = 0; segmentIndex < currentSegment.length; segmentIndex++) {
            result.push(currentSegment[segmentIndex]);
          }
          currentSegment = [];
        }
        result.push(modifier);
      } else {
        currentSegment.push(modifier);
      }
    }
    if (currentSegment.length > 0) {
      currentSegment.sort();
      for (let segmentIndex = 0; segmentIndex < currentSegment.length; segmentIndex++) {
        result.push(currentSegment[segmentIndex]);
      }
    }
    return result;
  };
};
var EXTERNAL_DESCRIPTOR = { isExternal: true, classId: -1, conflictIds: [] };
var DESCRIPTOR_CACHE_SIZE = 4096;
var MAX_CONFLICT_KEYS = 16384;
var createConfigUtils = (config) => {
  const sortModifiers = createSortModifiers(config);
  const postfixLookupClassGroupIds = createPostfixLookupClassGroupIds(config);
  const { getClassGroupId, getConflictingClassGroupIds } = createClassGroupUtils(config);
  let descriptorCache = /* @__PURE__ */ Object.create(null);
  let previousDescriptorCache = /* @__PURE__ */ Object.create(null);
  let descriptorCacheSize = 0;
  let claimedGeneration = new Int32Array(256);
  let currentGeneration = 0;
  let keepFlags = new Uint8Array(64);
  let splitSawNonSpaceWhitespace = false;
  const splitClassList = (classList) => {
    const tokens = [];
    const length = classList.length;
    let tokenStart = -1;
    splitSawNonSpaceWhitespace = false;
    for (let index = 0; index < length; index++) {
      const charCode = classList.charCodeAt(index);
      if (charCode === 32) {
        if (tokenStart !== -1) {
          tokens.push(classList.slice(tokenStart, index));
          tokenStart = -1;
        }
      } else if (charCode >= 9 && charCode <= 13) {
        splitSawNonSpaceWhitespace = true;
        if (tokenStart !== -1) {
          tokens.push(classList.slice(tokenStart, index));
          tokenStart = -1;
        }
      } else if (tokenStart === -1) {
        tokenStart = index;
      }
    }
    if (tokenStart !== -1) {
      tokens.push(classList.slice(tokenStart));
    }
    return tokens;
  };
  const conflictKeyIds = /* @__PURE__ */ new Map();
  let nextConflictKeyId = 0;
  const internConflictKey = (conflictKey) => {
    let id = conflictKeyIds.get(conflictKey);
    if (id === void 0) {
      id = nextConflictKeyId++;
      conflictKeyIds.set(conflictKey, id);
      if (id >= claimedGeneration.length) {
        const grown = new Int32Array(claimedGeneration.length * 2);
        grown.set(claimedGeneration);
        claimedGeneration = grown;
      }
    }
    return id;
  };
  const computeClassDescriptor = (originalClassName) => {
    const {
      isExternal,
      modifiers,
      hasImportantModifier,
      baseClassName,
      maybePostfixModifierPosition
    } = parseClassName(originalClassName);
    if (isExternal) {
      return EXTERNAL_DESCRIPTOR;
    }
    let hasPostfixModifier = Boolean(maybePostfixModifierPosition);
    let classGroupId;
    if (hasPostfixModifier) {
      const baseClassNameWithoutPostfix = baseClassName.substring(0, maybePostfixModifierPosition);
      classGroupId = getClassGroupId(baseClassNameWithoutPostfix);
      const classGroupIdWithPostfix = classGroupId && postfixLookupClassGroupIds[classGroupId] ? getClassGroupId(baseClassName) : void 0;
      if (classGroupIdWithPostfix && classGroupIdWithPostfix !== classGroupId) {
        classGroupId = classGroupIdWithPostfix;
        hasPostfixModifier = false;
      }
    } else {
      classGroupId = getClassGroupId(baseClassName);
    }
    if (!classGroupId) {
      if (!hasPostfixModifier) {
        return EXTERNAL_DESCRIPTOR;
      }
      classGroupId = getClassGroupId(baseClassName);
      if (!classGroupId) {
        return EXTERNAL_DESCRIPTOR;
      }
      hasPostfixModifier = false;
    }
    const variantModifier = modifiers.length === 0 ? "" : modifiers.length === 1 ? modifiers[0] : sortModifiers(modifiers).join(":");
    const modifierId = hasImportantModifier ? variantModifier + IMPORTANT_MODIFIER : variantModifier;
    const conflictGroups = getConflictingClassGroupIds(classGroupId, hasPostfixModifier);
    const conflictIds = [];
    for (let index = 0; index < conflictGroups.length; index++) {
      conflictIds.push(internConflictKey(modifierId + conflictGroups[index]));
    }
    return {
      isExternal: false,
      classId: internConflictKey(modifierId + classGroupId),
      conflictIds
    };
  };
  const getClassDescriptor = (originalClassName) => {
    let descriptor = descriptorCache[originalClassName];
    if (descriptor !== void 0) {
      return descriptor;
    }
    descriptor = previousDescriptorCache[originalClassName];
    if (descriptor === void 0) {
      descriptor = computeClassDescriptor(originalClassName);
    }
    descriptorCache[originalClassName] = descriptor;
    if (++descriptorCacheSize > DESCRIPTOR_CACHE_SIZE) {
      descriptorCacheSize = 0;
      previousDescriptorCache = descriptorCache;
      descriptorCache = /* @__PURE__ */ Object.create(null);
    }
    return descriptor;
  };
  const mergeClassList2 = (classList) => {
    const classNames = splitClassList(classList);
    const classCount = classNames.length;
    if (classCount === 1) {
      return classNames[0];
    }
    if (nextConflictKeyId > MAX_CONFLICT_KEYS) {
      conflictKeyIds.clear();
      nextConflictKeyId = 0;
      descriptorCache = /* @__PURE__ */ Object.create(null);
      previousDescriptorCache = /* @__PURE__ */ Object.create(null);
      descriptorCacheSize = 0;
    }
    currentGeneration = currentGeneration + 1 | 0;
    if (currentGeneration === 0) currentGeneration = 1;
    const generation = currentGeneration;
    if (classCount > keepFlags.length) {
      let capacity = keepFlags.length;
      while (capacity < classCount) capacity *= 2;
      keepFlags = new Uint8Array(capacity);
    }
    let didDrop = false;
    let tokenCharCount = 0;
    for (let index = classCount - 1; index >= 0; index -= 1) {
      const className = classNames[index];
      tokenCharCount += className.length;
      const descriptor = getClassDescriptor(className);
      if (descriptor.isExternal) {
        keepFlags[index] = 1;
        continue;
      }
      const classId = descriptor.classId;
      if (claimedGeneration[classId] === generation) {
        keepFlags[index] = 0;
        didDrop = true;
        continue;
      }
      claimedGeneration[classId] = generation;
      const conflictIds = descriptor.conflictIds;
      for (let conflictIndex = 0; conflictIndex < conflictIds.length; conflictIndex++) {
        claimedGeneration[conflictIds[conflictIndex]] = generation;
      }
      keepFlags[index] = 1;
    }
    if (!didDrop && !splitSawNonSpaceWhitespace && classList.length === tokenCharCount + classCount - 1) {
      return classList;
    }
    let result = "";
    for (let index = 0; index < classCount; index++) {
      if (keepFlags[index] === 1) {
        if (result) result += " ";
        result += classNames[index];
      }
    }
    return result;
  };
  return {
    parseClassName,
    sortModifiers,
    postfixLookupClassGroupIds,
    getClassGroupId,
    getConflictingClassGroupIds,
    getClassDescriptor,
    mergeClassList: mergeClassList2
  };
};
var createPostfixLookupClassGroupIds = (config) => {
  const lookup = /* @__PURE__ */ Object.create(null);
  const classGroupIds = config.postfixLookupClassGroups;
  if (classGroupIds) {
    for (let index = 0; index < classGroupIds.length; index++) {
      lookup[classGroupIds[index]] = true;
    }
  }
  return lookup;
};
var MERGE_CACHE_SIZE = 500;
var createTailwindMerge = (createConfig) => {
  let configUtils;
  let mergeClassList2;
  let cache = /* @__PURE__ */ Object.create(null);
  let previousCache = /* @__PURE__ */ Object.create(null);
  let cacheSize = 0;
  const initTailwindMerge = (classList) => {
    configUtils = createConfigUtils(createConfig());
    mergeClassList2 = configUtils.mergeClassList;
    merge.mergeString = tailwindMerge;
    return tailwindMerge(classList);
  };
  const tailwindMerge = (classList) => {
    let result = cache[classList];
    if (result !== void 0) {
      return result;
    }
    result = previousCache[classList];
    if (result === void 0) {
      result = mergeClassList2(classList);
    }
    cache[classList] = result;
    if (++cacheSize > MERGE_CACHE_SIZE) {
      cacheSize = 0;
      previousCache = cache;
      cache = /* @__PURE__ */ Object.create(null);
    }
    return result;
  };
  const merge = (...args) => merge.mergeString(joinClassValue(args));
  merge.mergeString = initTailwindMerge;
  return merge;
};
var fallbackThemeArr = [];
var fromTheme = (key) => {
  const themeGetter = (theme) => theme[key] || fallbackThemeArr;
  themeGetter.isThemeGetter = true;
  return themeGetter;
};
var arbitraryValueRegex = /^\[(?:(\w[\w-]*):)?(.+)\]$/i;
var arbitraryVariableRegex = /^\((?:(\w[\w-]*):)?(.+)\)$/i;
var fractionRegex = /^\d+(?:\.\d+)?\/\d+(?:\.\d+)?$/;
var tshirtUnitRegex = /^(\d+(\.\d+)?)?(xs|sm|md|lg|xl)$/;
var lengthUnitRegex = /\d+(%|px|r?em|[sdl]?v([hwib]|min|max)|pt|pc|in|cm|mm|cap|ch|ex|r?lh|cq(w|h|i|b|min|max))|\b(calc|min|max|clamp)\(.+\)|^0$/;
var colorFunctionRegex = /^(rgba?|hsla?|hwb|(ok)?(lab|lch)|color-mix)\(.+\)$/;
var shadowRegex = /^(inset_)?-?((\d+)?\.?(\d+)[a-z]+|0)_-?((\d+)?\.?(\d+)[a-z]+|0)/;
var imageRegex = /^(url|image|image-set|cross-fade|element|(repeating-)?(linear|radial|conic)-gradient)\(.+\)$/;
var toNumber = Number;
var numberIsNaN = Number.isNaN;
var numberIsInteger = Number.isInteger;
var isFraction = (value) => fractionRegex.test(value);
var isNumber = (value) => Boolean(value) && !numberIsNaN(toNumber(value));
var isInteger = (value) => Boolean(value) && numberIsInteger(toNumber(value));
var isPercent = (value) => value.endsWith("%") && isNumber(value.slice(0, -1));
var isTshirtSize = (value) => tshirtUnitRegex.test(value);
var isAny = () => true;
var isLengthOnly = (value) => (
  // `colorFunctionRegex` check is necessary because color functions can have percentages in them which which would be incorrectly classified as lengths.
  // For example, `hsl(0 0% 0%)` would be classified as a length without this check.
  // I could also use lookbehind assertion in `lengthUnitRegex` but that isn't supported widely enough.
  lengthUnitRegex.test(value) && !colorFunctionRegex.test(value)
);
var isNever = () => false;
var isShadow = (value) => shadowRegex.test(value);
var isImage = (value) => imageRegex.test(value);
var isAnyNonArbitrary = (value) => !isArbitraryValue(value) && !isArbitraryVariable(value);
var isNamedContainerQuery = (value) => value.startsWith("@container") && (value[10] === "/" && value[11] !== void 0 || value[11] === "s" && value[16] !== void 0 && value.startsWith("-size/", 10) || value[11] === "n" && value[18] !== void 0 && value.startsWith("-normal/", 10));
var isArbitrarySize = (value) => getIsArbitraryValue(value, isLabelSize, isNever);
var isArbitraryValue = (value) => arbitraryValueRegex.test(value);
var isArbitraryLength = (value) => getIsArbitraryValue(value, isLabelLength, isLengthOnly);
var isArbitraryNumber = (value) => getIsArbitraryValue(value, isLabelNumber, isNumber);
var isArbitraryWeight = (value) => getIsArbitraryValue(value, isLabelWeight, isAny);
var isArbitraryFamilyName = (value) => getIsArbitraryValue(value, isLabelFamilyName, isNever);
var isArbitraryPosition = (value) => getIsArbitraryValue(value, isLabelPosition, isNever);
var isArbitraryImage = (value) => getIsArbitraryValue(value, isLabelImage, isImage);
var isArbitraryShadow = (value) => getIsArbitraryValue(value, isLabelShadow, isShadow);
var isArbitraryVariable = (value) => arbitraryVariableRegex.test(value);
var isArbitraryVariableLength = (value) => getIsArbitraryVariable(value, isLabelLength);
var isArbitraryVariableFamilyName = (value) => getIsArbitraryVariable(value, isLabelFamilyName);
var isArbitraryVariablePosition = (value) => getIsArbitraryVariable(value, isLabelPosition);
var isArbitraryVariableSize = (value) => getIsArbitraryVariable(value, isLabelSize);
var isArbitraryVariableImage = (value) => getIsArbitraryVariable(value, isLabelImage);
var isArbitraryVariableShadow = (value) => getIsArbitraryVariable(value, isLabelShadow, true);
var isArbitraryVariableWeight = (value) => getIsArbitraryVariable(value, isLabelWeight, true);
var getIsArbitraryValue = (value, testLabel, testValue) => {
  const result = arbitraryValueRegex.exec(value);
  if (result) {
    if (result[1]) {
      return testLabel(result[1]);
    }
    return testValue(result[2]);
  }
  return false;
};
var getIsArbitraryVariable = (value, testLabel, shouldMatchNoLabel = false) => {
  const result = arbitraryVariableRegex.exec(value);
  if (result) {
    if (result[1]) {
      return testLabel(result[1]);
    }
    return shouldMatchNoLabel;
  }
  return false;
};
var isLabelPosition = (label) => label === "position" || label === "percentage";
var isLabelImage = (label) => label === "image" || label === "url";
var isLabelSize = (label) => label === "length" || label === "size" || label === "bg-size";
var isLabelLength = (label) => label === "length";
var isLabelNumber = (label) => label === "number";
var isLabelFamilyName = (label) => label === "family-name";
var isLabelWeight = (label) => label === "number" || label === "weight";
var isLabelShadow = (label) => label === "shadow";
var getDefaultConfig = () => {
  const themeColor = fromTheme("color");
  const themeFont = fromTheme("font");
  const themeText = fromTheme("text");
  const themeFontWeight = fromTheme("font-weight");
  const themeTracking = fromTheme("tracking");
  const themeLeading = fromTheme("leading");
  const themeBreakpoint = fromTheme("breakpoint");
  const themeContainer = fromTheme("container");
  const themeSpacing = fromTheme("spacing");
  const themeRadius = fromTheme("radius");
  const themeShadow = fromTheme("shadow");
  const themeInsetShadow = fromTheme("inset-shadow");
  const themeTextShadow = fromTheme("text-shadow");
  const themeDropShadow = fromTheme("drop-shadow");
  const themeBlur = fromTheme("blur");
  const themePerspective = fromTheme("perspective");
  const themeAspect = fromTheme("aspect");
  const themeEase = fromTheme("ease");
  const themeAnimate = fromTheme("animate");
  const scaleBreak = () => ["auto", "avoid", "all", "avoid-page", "page", "left", "right", "column"];
  const scalePosition = () => [
    "center",
    "top",
    "bottom",
    "left",
    "right",
    "top-left",
    // Deprecated since Tailwind CSS v4.1.0, see https://github.com/tailwindlabs/tailwindcss/pull/17378
    "left-top",
    "top-right",
    // Deprecated since Tailwind CSS v4.1.0, see https://github.com/tailwindlabs/tailwindcss/pull/17378
    "right-top",
    "bottom-right",
    // Deprecated since Tailwind CSS v4.1.0, see https://github.com/tailwindlabs/tailwindcss/pull/17378
    "right-bottom",
    "bottom-left",
    // Deprecated since Tailwind CSS v4.1.0, see https://github.com/tailwindlabs/tailwindcss/pull/17378
    "left-bottom"
  ];
  const scalePositionWithArbitrary = () => [...scalePosition(), isArbitraryVariable, isArbitraryValue];
  const scaleOverflow = () => ["auto", "hidden", "clip", "visible", "scroll"];
  const scaleOverscroll = () => ["auto", "contain", "none"];
  const scaleUnambiguousSpacing = () => [isArbitraryVariable, isArbitraryValue, themeSpacing];
  const scaleInset = () => [isFraction, "full", "auto", ...scaleUnambiguousSpacing()];
  const scaleGridTemplateColsRows = () => [isInteger, "none", "subgrid", isArbitraryVariable, isArbitraryValue];
  const scaleGridColRowStartAndEnd = () => [
    "auto",
    { span: ["full", isInteger, isArbitraryVariable, isArbitraryValue] },
    isInteger,
    isArbitraryVariable,
    isArbitraryValue
  ];
  const scaleGridColRowStartOrEnd = () => [isInteger, "auto", isArbitraryVariable, isArbitraryValue];
  const scaleGridAutoColsRows = () => ["auto", "min", "max", "fr", isArbitraryVariable, isArbitraryValue];
  const scaleAlignPrimaryAxis = () => [
    "start",
    "end",
    "center",
    "between",
    "around",
    "evenly",
    "stretch",
    "baseline",
    "center-safe",
    "end-safe"
  ];
  const scaleAlignSecondaryAxis = () => ["start", "end", "center", "stretch", "center-safe", "end-safe"];
  const scaleMargin = () => ["auto", ...scaleUnambiguousSpacing()];
  const scaleSizing = () => [
    isFraction,
    "auto",
    "full",
    "dvw",
    "dvh",
    "lvw",
    "lvh",
    "svw",
    "svh",
    "min",
    "max",
    "fit",
    ...scaleUnambiguousSpacing()
  ];
  const scaleSizingInline = () => [
    isFraction,
    "screen",
    "full",
    "dvw",
    "lvw",
    "svw",
    "min",
    "max",
    "fit",
    ...scaleUnambiguousSpacing()
  ];
  const scaleSizingBlock = () => [
    isFraction,
    "screen",
    "full",
    "lh",
    "dvh",
    "lvh",
    "svh",
    "min",
    "max",
    "fit",
    ...scaleUnambiguousSpacing()
  ];
  const scaleColor = () => [themeColor, isArbitraryVariable, isArbitraryValue];
  const scaleBgPosition = () => [
    ...scalePosition(),
    isArbitraryVariablePosition,
    isArbitraryPosition,
    { position: [isArbitraryVariable, isArbitraryValue] }
  ];
  const scaleBgRepeat = () => ["no-repeat", { repeat: ["", "x", "y", "space", "round"] }];
  const scaleBgSize = () => [
    "auto",
    "cover",
    "contain",
    isArbitraryVariableSize,
    isArbitrarySize,
    { size: [isArbitraryVariable, isArbitraryValue] }
  ];
  const scaleGradientStopPosition = () => [isPercent, isArbitraryVariableLength, isArbitraryLength];
  const scaleRadius = () => [
    // Deprecated since Tailwind CSS v4.0.0
    "",
    "none",
    "full",
    themeRadius,
    isArbitraryVariable,
    isArbitraryValue
  ];
  const scaleBorderWidth = () => ["", isNumber, isArbitraryVariableLength, isArbitraryLength];
  const scaleLineStyle = () => ["solid", "dashed", "dotted", "double"];
  const scaleBlendMode = () => [
    "normal",
    "multiply",
    "screen",
    "overlay",
    "darken",
    "lighten",
    "color-dodge",
    "color-burn",
    "hard-light",
    "soft-light",
    "difference",
    "exclusion",
    "hue",
    "saturation",
    "color",
    "luminosity"
  ];
  const scaleMaskImagePosition = () => [isNumber, isPercent, isArbitraryVariablePosition, isArbitraryPosition];
  const scaleBlur = () => [
    // Deprecated since Tailwind CSS v4.0.0
    "",
    "none",
    themeBlur,
    isArbitraryVariable,
    isArbitraryValue
  ];
  const scaleRotate = () => ["none", isNumber, isArbitraryVariable, isArbitraryValue];
  const scaleScale = () => ["none", isNumber, isArbitraryVariable, isArbitraryValue];
  const scaleSkew = () => [isNumber, isArbitraryVariable, isArbitraryValue];
  const scaleTranslate = () => [isFraction, "full", ...scaleUnambiguousSpacing()];
  return {
    theme: {
      animate: ["spin", "ping", "pulse", "bounce"],
      aspect: ["video"],
      blur: [isTshirtSize],
      breakpoint: [isTshirtSize],
      color: [isAny],
      container: [isTshirtSize],
      "drop-shadow": [isTshirtSize],
      ease: ["in", "out", "in-out"],
      font: [isAnyNonArbitrary],
      "font-weight": [
        "thin",
        "extralight",
        "light",
        "normal",
        "medium",
        "semibold",
        "bold",
        "extrabold",
        "black"
      ],
      "inset-shadow": [isTshirtSize],
      leading: ["none", "tight", "snug", "normal", "relaxed", "loose"],
      perspective: ["dramatic", "near", "normal", "midrange", "distant", "none"],
      radius: [isTshirtSize],
      shadow: [isTshirtSize],
      spacing: ["px", isNumber],
      text: [isTshirtSize],
      "text-shadow": [isTshirtSize],
      tracking: ["tighter", "tight", "normal", "wide", "wider", "widest"]
    },
    classGroups: {
      // --------------
      // --- Layout ---
      // --------------
      /**
       * Aspect Ratio
       * @see https://tailwindcss.com/docs/aspect-ratio
       */
      aspect: [
        {
          aspect: [
            "auto",
            "square",
            isFraction,
            isArbitraryValue,
            isArbitraryVariable,
            themeAspect
          ]
        }
      ],
      /**
       * Container
       * @see https://tailwindcss.com/docs/container
       * @deprecated since Tailwind CSS v4.0.0
       */
      container: ["container"],
      /**
       * Container Type
       * @see https://tailwindcss.com/docs/responsive-design#container-queries
       */
      "container-type": [
        { "@container": ["", "normal", "size", isArbitraryVariable, isArbitraryValue] }
      ],
      /**
       * Container Name
       * @see https://tailwindcss.com/docs/responsive-design#named-containers
       */
      "container-named": [isNamedContainerQuery],
      /**
       * Columns
       * @see https://tailwindcss.com/docs/columns
       */
      columns: [{ columns: [isNumber, isArbitraryValue, isArbitraryVariable, themeContainer] }],
      /**
       * Break After
       * @see https://tailwindcss.com/docs/break-after
       */
      "break-after": [{ "break-after": scaleBreak() }],
      /**
       * Break Before
       * @see https://tailwindcss.com/docs/break-before
       */
      "break-before": [{ "break-before": scaleBreak() }],
      /**
       * Break Inside
       * @see https://tailwindcss.com/docs/break-inside
       */
      "break-inside": [{ "break-inside": ["auto", "avoid", "avoid-page", "avoid-column"] }],
      /**
       * Box Decoration Break
       * @see https://tailwindcss.com/docs/box-decoration-break
       */
      "box-decoration": [{ "box-decoration": ["slice", "clone"] }],
      /**
       * Box Sizing
       * @see https://tailwindcss.com/docs/box-sizing
       */
      box: [{ box: ["border", "content"] }],
      /**
       * Display
       * @see https://tailwindcss.com/docs/display
       */
      display: [
        "block",
        "inline-block",
        "inline",
        "flex",
        "inline-flex",
        "table",
        "inline-table",
        "table-caption",
        "table-cell",
        "table-column",
        "table-column-group",
        "table-footer-group",
        "table-header-group",
        "table-row-group",
        "table-row",
        "flow-root",
        "grid",
        "inline-grid",
        "contents",
        "list-item",
        "hidden"
      ],
      /**
       * Screen Reader Only
       * @see https://tailwindcss.com/docs/display#screen-reader-only
       */
      sr: ["sr-only", "not-sr-only"],
      /**
       * Floats
       * @see https://tailwindcss.com/docs/float
       */
      float: [{ float: ["right", "left", "none", "start", "end"] }],
      /**
       * Clear
       * @see https://tailwindcss.com/docs/clear
       */
      clear: [{ clear: ["left", "right", "both", "none", "start", "end"] }],
      /**
       * Isolation
       * @see https://tailwindcss.com/docs/isolation
       */
      isolation: ["isolate", "isolation-auto"],
      /**
       * Object Fit
       * @see https://tailwindcss.com/docs/object-fit
       */
      "object-fit": [{ object: ["contain", "cover", "fill", "none", "scale-down"] }],
      /**
       * Object Position
       * @see https://tailwindcss.com/docs/object-position
       */
      "object-position": [{ object: scalePositionWithArbitrary() }],
      /**
       * Overflow
       * @see https://tailwindcss.com/docs/overflow
       */
      overflow: [{ overflow: scaleOverflow() }],
      /**
       * Overflow X
       * @see https://tailwindcss.com/docs/overflow
       */
      "overflow-x": [{ "overflow-x": scaleOverflow() }],
      /**
       * Overflow Y
       * @see https://tailwindcss.com/docs/overflow
       */
      "overflow-y": [{ "overflow-y": scaleOverflow() }],
      /**
       * Overscroll Behavior
       * @see https://tailwindcss.com/docs/overscroll-behavior
       */
      overscroll: [{ overscroll: scaleOverscroll() }],
      /**
       * Overscroll Behavior X
       * @see https://tailwindcss.com/docs/overscroll-behavior
       */
      "overscroll-x": [{ "overscroll-x": scaleOverscroll() }],
      /**
       * Overscroll Behavior Y
       * @see https://tailwindcss.com/docs/overscroll-behavior
       */
      "overscroll-y": [{ "overscroll-y": scaleOverscroll() }],
      /**
       * Position
       * @see https://tailwindcss.com/docs/position
       */
      position: ["static", "fixed", "absolute", "relative", "sticky"],
      /**
       * Inset
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      inset: [{ inset: scaleInset() }],
      /**
       * Inset Inline
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      "inset-x": [{ "inset-x": scaleInset() }],
      /**
       * Inset Block
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      "inset-y": [{ "inset-y": scaleInset() }],
      /**
       * Inset Inline Start
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       * @todo class group will be renamed to `inset-s` in next major release
       */
      start: [
        {
          "inset-s": scaleInset(),
          /**
           * @deprecated since Tailwind CSS v4.2.0 in favor of `inset-s-*` utilities.
           * @see https://github.com/tailwindlabs/tailwindcss/pull/19613
           */
          start: scaleInset()
        }
      ],
      /**
       * Inset Inline End
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       * @todo class group will be renamed to `inset-e` in next major release
       */
      end: [
        {
          "inset-e": scaleInset(),
          /**
           * @deprecated since Tailwind CSS v4.2.0 in favor of `inset-e-*` utilities.
           * @see https://github.com/tailwindlabs/tailwindcss/pull/19613
           */
          end: scaleInset()
        }
      ],
      /**
       * Inset Block Start
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      "inset-bs": [{ "inset-bs": scaleInset() }],
      /**
       * Inset Block End
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      "inset-be": [{ "inset-be": scaleInset() }],
      /**
       * Top
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      top: [{ top: scaleInset() }],
      /**
       * Right
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      right: [{ right: scaleInset() }],
      /**
       * Bottom
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      bottom: [{ bottom: scaleInset() }],
      /**
       * Left
       * @see https://tailwindcss.com/docs/top-right-bottom-left
       */
      left: [{ left: scaleInset() }],
      /**
       * Visibility
       * @see https://tailwindcss.com/docs/visibility
       */
      visibility: ["visible", "invisible", "collapse"],
      /**
       * Z-Index
       * @see https://tailwindcss.com/docs/z-index
       */
      z: [{ z: [isInteger, "auto", isArbitraryVariable, isArbitraryValue] }],
      // ------------------------
      // --- Flexbox and Grid ---
      // ------------------------
      /**
       * Flex Basis
       * @see https://tailwindcss.com/docs/flex-basis
       */
      basis: [
        {
          basis: [isFraction, "full", "auto", themeContainer, ...scaleUnambiguousSpacing()]
        }
      ],
      /**
       * Flex Direction
       * @see https://tailwindcss.com/docs/flex-direction
       */
      "flex-direction": [{ flex: ["row", "row-reverse", "col", "col-reverse"] }],
      /**
       * Flex Wrap
       * @see https://tailwindcss.com/docs/flex-wrap
       */
      "flex-wrap": [{ flex: ["nowrap", "wrap", "wrap-reverse"] }],
      /**
       * Flex
       * @see https://tailwindcss.com/docs/flex
       */
      flex: [{ flex: [isNumber, isFraction, "auto", "initial", "none", isArbitraryValue] }],
      /**
       * Flex Grow
       * @see https://tailwindcss.com/docs/flex-grow
       */
      grow: [{ grow: ["", isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Flex Shrink
       * @see https://tailwindcss.com/docs/flex-shrink
       */
      shrink: [{ shrink: ["", isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Order
       * @see https://tailwindcss.com/docs/order
       */
      order: [
        {
          order: [isInteger, "first", "last", "none", isArbitraryVariable, isArbitraryValue]
        }
      ],
      /**
       * Grid Template Columns
       * @see https://tailwindcss.com/docs/grid-template-columns
       */
      "grid-cols": [{ "grid-cols": scaleGridTemplateColsRows() }],
      /**
       * Grid Column Start / End
       * @see https://tailwindcss.com/docs/grid-column
       */
      "col-start-end": [{ col: scaleGridColRowStartAndEnd() }],
      /**
       * Grid Column Start
       * @see https://tailwindcss.com/docs/grid-column
       */
      "col-start": [{ "col-start": scaleGridColRowStartOrEnd() }],
      /**
       * Grid Column End
       * @see https://tailwindcss.com/docs/grid-column
       */
      "col-end": [{ "col-end": scaleGridColRowStartOrEnd() }],
      /**
       * Grid Template Rows
       * @see https://tailwindcss.com/docs/grid-template-rows
       */
      "grid-rows": [{ "grid-rows": scaleGridTemplateColsRows() }],
      /**
       * Grid Row Start / End
       * @see https://tailwindcss.com/docs/grid-row
       */
      "row-start-end": [{ row: scaleGridColRowStartAndEnd() }],
      /**
       * Grid Row Start
       * @see https://tailwindcss.com/docs/grid-row
       */
      "row-start": [{ "row-start": scaleGridColRowStartOrEnd() }],
      /**
       * Grid Row End
       * @see https://tailwindcss.com/docs/grid-row
       */
      "row-end": [{ "row-end": scaleGridColRowStartOrEnd() }],
      /**
       * Grid Auto Flow
       * @see https://tailwindcss.com/docs/grid-auto-flow
       */
      "grid-flow": [{ "grid-flow": ["row", "col", "dense", "row-dense", "col-dense"] }],
      /**
       * Grid Auto Columns
       * @see https://tailwindcss.com/docs/grid-auto-columns
       */
      "auto-cols": [{ "auto-cols": scaleGridAutoColsRows() }],
      /**
       * Grid Auto Rows
       * @see https://tailwindcss.com/docs/grid-auto-rows
       */
      "auto-rows": [{ "auto-rows": scaleGridAutoColsRows() }],
      /**
       * Gap
       * @see https://tailwindcss.com/docs/gap
       */
      gap: [{ gap: scaleUnambiguousSpacing() }],
      /**
       * Gap X
       * @see https://tailwindcss.com/docs/gap
       */
      "gap-x": [{ "gap-x": scaleUnambiguousSpacing() }],
      /**
       * Gap Y
       * @see https://tailwindcss.com/docs/gap
       */
      "gap-y": [{ "gap-y": scaleUnambiguousSpacing() }],
      /**
       * Justify Content
       * @see https://tailwindcss.com/docs/justify-content
       */
      "justify-content": [{ justify: [...scaleAlignPrimaryAxis(), "normal"] }],
      /**
       * Justify Items
       * @see https://tailwindcss.com/docs/justify-items
       */
      "justify-items": [{ "justify-items": [...scaleAlignSecondaryAxis(), "normal"] }],
      /**
       * Justify Self
       * @see https://tailwindcss.com/docs/justify-self
       */
      "justify-self": [{ "justify-self": ["auto", ...scaleAlignSecondaryAxis()] }],
      /**
       * Align Content
       * @see https://tailwindcss.com/docs/align-content
       */
      "align-content": [{ content: ["normal", ...scaleAlignPrimaryAxis()] }],
      /**
       * Align Items
       * @see https://tailwindcss.com/docs/align-items
       */
      "align-items": [{ items: [...scaleAlignSecondaryAxis(), { baseline: ["", "last"] }] }],
      /**
       * Align Self
       * @see https://tailwindcss.com/docs/align-self
       */
      "align-self": [{ self: ["auto", ...scaleAlignSecondaryAxis(), { baseline: ["", "last"] }] }],
      /**
       * Place Content
       * @see https://tailwindcss.com/docs/place-content
       */
      "place-content": [{ "place-content": scaleAlignPrimaryAxis() }],
      /**
       * Place Items
       * @see https://tailwindcss.com/docs/place-items
       */
      "place-items": [{ "place-items": [...scaleAlignSecondaryAxis(), "baseline"] }],
      /**
       * Place Self
       * @see https://tailwindcss.com/docs/place-self
       */
      "place-self": [{ "place-self": ["auto", ...scaleAlignSecondaryAxis()] }],
      // Spacing
      /**
       * Padding
       * @see https://tailwindcss.com/docs/padding
       */
      p: [{ p: scaleUnambiguousSpacing() }],
      /**
       * Padding Inline
       * @see https://tailwindcss.com/docs/padding
       */
      px: [{ px: scaleUnambiguousSpacing() }],
      /**
       * Padding Block
       * @see https://tailwindcss.com/docs/padding
       */
      py: [{ py: scaleUnambiguousSpacing() }],
      /**
       * Padding Inline Start
       * @see https://tailwindcss.com/docs/padding
       */
      ps: [{ ps: scaleUnambiguousSpacing() }],
      /**
       * Padding Inline End
       * @see https://tailwindcss.com/docs/padding
       */
      pe: [{ pe: scaleUnambiguousSpacing() }],
      /**
       * Padding Block Start
       * @see https://tailwindcss.com/docs/padding
       */
      pbs: [{ pbs: scaleUnambiguousSpacing() }],
      /**
       * Padding Block End
       * @see https://tailwindcss.com/docs/padding
       */
      pbe: [{ pbe: scaleUnambiguousSpacing() }],
      /**
       * Padding Top
       * @see https://tailwindcss.com/docs/padding
       */
      pt: [{ pt: scaleUnambiguousSpacing() }],
      /**
       * Padding Right
       * @see https://tailwindcss.com/docs/padding
       */
      pr: [{ pr: scaleUnambiguousSpacing() }],
      /**
       * Padding Bottom
       * @see https://tailwindcss.com/docs/padding
       */
      pb: [{ pb: scaleUnambiguousSpacing() }],
      /**
       * Padding Left
       * @see https://tailwindcss.com/docs/padding
       */
      pl: [{ pl: scaleUnambiguousSpacing() }],
      /**
       * Margin
       * @see https://tailwindcss.com/docs/margin
       */
      m: [{ m: scaleMargin() }],
      /**
       * Margin Inline
       * @see https://tailwindcss.com/docs/margin
       */
      mx: [{ mx: scaleMargin() }],
      /**
       * Margin Block
       * @see https://tailwindcss.com/docs/margin
       */
      my: [{ my: scaleMargin() }],
      /**
       * Margin Inline Start
       * @see https://tailwindcss.com/docs/margin
       */
      ms: [{ ms: scaleMargin() }],
      /**
       * Margin Inline End
       * @see https://tailwindcss.com/docs/margin
       */
      me: [{ me: scaleMargin() }],
      /**
       * Margin Block Start
       * @see https://tailwindcss.com/docs/margin
       */
      mbs: [{ mbs: scaleMargin() }],
      /**
       * Margin Block End
       * @see https://tailwindcss.com/docs/margin
       */
      mbe: [{ mbe: scaleMargin() }],
      /**
       * Margin Top
       * @see https://tailwindcss.com/docs/margin
       */
      mt: [{ mt: scaleMargin() }],
      /**
       * Margin Right
       * @see https://tailwindcss.com/docs/margin
       */
      mr: [{ mr: scaleMargin() }],
      /**
       * Margin Bottom
       * @see https://tailwindcss.com/docs/margin
       */
      mb: [{ mb: scaleMargin() }],
      /**
       * Margin Left
       * @see https://tailwindcss.com/docs/margin
       */
      ml: [{ ml: scaleMargin() }],
      /**
       * Space Between X
       * @see https://tailwindcss.com/docs/margin#adding-space-between-children
       */
      "space-x": [{ "space-x": scaleUnambiguousSpacing() }],
      /**
       * Space Between X Reverse
       * @see https://tailwindcss.com/docs/margin#adding-space-between-children
       */
      "space-x-reverse": ["space-x-reverse"],
      /**
       * Space Between Y
       * @see https://tailwindcss.com/docs/margin#adding-space-between-children
       */
      "space-y": [{ "space-y": scaleUnambiguousSpacing() }],
      /**
       * Space Between Y Reverse
       * @see https://tailwindcss.com/docs/margin#adding-space-between-children
       */
      "space-y-reverse": ["space-y-reverse"],
      // --------------
      // --- Sizing ---
      // --------------
      /**
       * Size
       * @see https://tailwindcss.com/docs/width#setting-both-width-and-height
       */
      size: [{ size: scaleSizing() }],
      /**
       * Inline Size
       * @see https://tailwindcss.com/docs/width
       */
      "inline-size": [{ inline: ["auto", ...scaleSizingInline()] }],
      /**
       * Min-Inline Size
       * @see https://tailwindcss.com/docs/min-width
       */
      "min-inline-size": [{ "min-inline": ["auto", ...scaleSizingInline()] }],
      /**
       * Max-Inline Size
       * @see https://tailwindcss.com/docs/max-width
       */
      "max-inline-size": [{ "max-inline": ["none", ...scaleSizingInline()] }],
      /**
       * Block Size
       * @see https://tailwindcss.com/docs/height
       */
      "block-size": [{ block: ["auto", ...scaleSizingBlock()] }],
      /**
       * Min-Block Size
       * @see https://tailwindcss.com/docs/min-height
       */
      "min-block-size": [{ "min-block": ["auto", ...scaleSizingBlock()] }],
      /**
       * Max-Block Size
       * @see https://tailwindcss.com/docs/max-height
       */
      "max-block-size": [{ "max-block": ["none", ...scaleSizingBlock()] }],
      /**
       * Width
       * @see https://tailwindcss.com/docs/width
       */
      w: [{ w: [themeContainer, "screen", ...scaleSizing()] }],
      /**
       * Min-Width
       * @see https://tailwindcss.com/docs/min-width
       */
      "min-w": [
        {
          "min-w": [
            themeContainer,
            "screen",
            /** Deprecated. @see https://github.com/tailwindlabs/tailwindcss.com/issues/2027#issuecomment-2620152757 */
            "none",
            ...scaleSizing()
          ]
        }
      ],
      /**
       * Max-Width
       * @see https://tailwindcss.com/docs/max-width
       */
      "max-w": [
        {
          "max-w": [
            themeContainer,
            "screen",
            "none",
            /** Deprecated since Tailwind CSS v4.0.0. @see https://github.com/tailwindlabs/tailwindcss.com/issues/2027#issuecomment-2620152757 */
            "prose",
            /** Deprecated since Tailwind CSS v4.0.0. @see https://github.com/tailwindlabs/tailwindcss.com/issues/2027#issuecomment-2620152757 */
            { screen: [themeBreakpoint] },
            ...scaleSizing()
          ]
        }
      ],
      /**
       * Height
       * @see https://tailwindcss.com/docs/height
       */
      h: [{ h: ["screen", "lh", ...scaleSizing()] }],
      /**
       * Min-Height
       * @see https://tailwindcss.com/docs/min-height
       */
      "min-h": [{ "min-h": ["screen", "lh", "none", ...scaleSizing()] }],
      /**
       * Max-Height
       * @see https://tailwindcss.com/docs/max-height
       */
      "max-h": [{ "max-h": ["screen", "lh", ...scaleSizing()] }],
      // ------------------
      // --- Typography ---
      // ------------------
      /**
       * Font Size
       * @see https://tailwindcss.com/docs/font-size
       */
      "font-size": [{ text: ["base", themeText, isArbitraryVariableLength, isArbitraryLength] }],
      /**
       * Font Smoothing
       * @see https://tailwindcss.com/docs/font-smoothing
       */
      "font-smoothing": ["antialiased", "subpixel-antialiased"],
      /**
       * Font Style
       * @see https://tailwindcss.com/docs/font-style
       */
      "font-style": ["italic", "not-italic"],
      /**
       * Font Weight
       * @see https://tailwindcss.com/docs/font-weight
       */
      "font-weight": [
        {
          font: [themeFontWeight, isArbitraryVariableWeight, isArbitraryWeight]
        }
      ],
      /**
       * Font Stretch
       * @see https://tailwindcss.com/docs/font-stretch
       */
      "font-stretch": [
        {
          "font-stretch": [
            "ultra-condensed",
            "extra-condensed",
            "condensed",
            "semi-condensed",
            "normal",
            "semi-expanded",
            "expanded",
            "extra-expanded",
            "ultra-expanded",
            isPercent,
            isArbitraryValue
          ]
        }
      ],
      /**
       * Font Family
       * @see https://tailwindcss.com/docs/font-family
       */
      "font-family": [{ font: [isArbitraryVariableFamilyName, isArbitraryFamilyName, themeFont] }],
      /**
       * Font Feature Settings
       * @see https://tailwindcss.com/docs/font-feature-settings
       */
      "font-features": [{ "font-features": [isArbitraryValue] }],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-normal": ["normal-nums"],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-ordinal": ["ordinal"],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-slashed-zero": ["slashed-zero"],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-figure": ["lining-nums", "oldstyle-nums"],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-spacing": ["proportional-nums", "tabular-nums"],
      /**
       * Font Variant Numeric
       * @see https://tailwindcss.com/docs/font-variant-numeric
       */
      "fvn-fraction": ["diagonal-fractions", "stacked-fractions"],
      /**
       * Letter Spacing
       * @see https://tailwindcss.com/docs/letter-spacing
       */
      tracking: [{ tracking: [themeTracking, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Line Clamp
       * @see https://tailwindcss.com/docs/line-clamp
       */
      "line-clamp": [{ "line-clamp": [isNumber, "none", isArbitraryVariable, isArbitraryNumber] }],
      /**
       * Line Height
       * @see https://tailwindcss.com/docs/line-height
       */
      leading: [
        {
          leading: [
            /** Deprecated since Tailwind CSS v4.0.0. @see https://github.com/tailwindlabs/tailwindcss.com/issues/2027#issuecomment-2620152757 */
            themeLeading,
            ...scaleUnambiguousSpacing()
          ]
        }
      ],
      /**
       * List Style Image
       * @see https://tailwindcss.com/docs/list-style-image
       */
      "list-image": [{ "list-image": ["none", isArbitraryVariable, isArbitraryValue] }],
      /**
       * List Style Position
       * @see https://tailwindcss.com/docs/list-style-position
       */
      "list-style-position": [{ list: ["inside", "outside"] }],
      /**
       * List Style Type
       * @see https://tailwindcss.com/docs/list-style-type
       */
      "list-style-type": [
        { list: ["disc", "decimal", "none", isArbitraryVariable, isArbitraryValue] }
      ],
      /**
       * Text Alignment
       * @see https://tailwindcss.com/docs/text-align
       */
      "text-alignment": [{ text: ["left", "center", "right", "justify", "start", "end"] }],
      /**
       * Placeholder Color
       * @deprecated since Tailwind CSS v3.0.0
       * @see https://v3.tailwindcss.com/docs/placeholder-color
       */
      "placeholder-color": [{ placeholder: scaleColor() }],
      /**
       * Text Color
       * @see https://tailwindcss.com/docs/text-color
       */
      "text-color": [{ text: scaleColor() }],
      /**
       * Text Decoration
       * @see https://tailwindcss.com/docs/text-decoration
       */
      "text-decoration": ["underline", "overline", "line-through", "no-underline"],
      /**
       * Text Decoration Style
       * @see https://tailwindcss.com/docs/text-decoration-style
       */
      "text-decoration-style": [{ decoration: [...scaleLineStyle(), "wavy"] }],
      /**
       * Text Decoration Thickness
       * @see https://tailwindcss.com/docs/text-decoration-thickness
       */
      "text-decoration-thickness": [
        {
          decoration: [isNumber, "from-font", "auto", isArbitraryVariable, isArbitraryLength]
        }
      ],
      /**
       * Text Decoration Color
       * @see https://tailwindcss.com/docs/text-decoration-color
       */
      "text-decoration-color": [{ decoration: scaleColor() }],
      /**
       * Text Underline Offset
       * @see https://tailwindcss.com/docs/text-underline-offset
       */
      "underline-offset": [
        { "underline-offset": [isNumber, "auto", isArbitraryVariable, isArbitraryValue] }
      ],
      /**
       * Text Transform
       * @see https://tailwindcss.com/docs/text-transform
       */
      "text-transform": ["uppercase", "lowercase", "capitalize", "normal-case"],
      /**
       * Text Overflow
       * @see https://tailwindcss.com/docs/text-overflow
       */
      "text-overflow": ["truncate", "text-ellipsis", "text-clip"],
      /**
       * Text Wrap
       * @see https://tailwindcss.com/docs/text-wrap
       */
      "text-wrap": [{ text: ["wrap", "nowrap", "balance", "pretty"] }],
      /**
       * Text Indent
       * @see https://tailwindcss.com/docs/text-indent
       */
      indent: [{ indent: scaleUnambiguousSpacing() }],
      /**
       * Tab Size
       * @see https://tailwindcss.com/docs/tab-size
       */
      "tab-size": [{ tab: [isInteger, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Vertical Alignment
       * @see https://tailwindcss.com/docs/vertical-align
       */
      "vertical-align": [
        {
          align: [
            "baseline",
            "top",
            "middle",
            "bottom",
            "text-top",
            "text-bottom",
            "sub",
            "super",
            isArbitraryVariable,
            isArbitraryValue
          ]
        }
      ],
      /**
       * Whitespace
       * @see https://tailwindcss.com/docs/whitespace
       */
      whitespace: [
        { whitespace: ["normal", "nowrap", "pre", "pre-line", "pre-wrap", "break-spaces"] }
      ],
      /**
       * Word Break
       * @see https://tailwindcss.com/docs/word-break
       */
      break: [{ break: ["normal", "words", "all", "keep"] }],
      /**
       * Overflow Wrap
       * @see https://tailwindcss.com/docs/overflow-wrap
       */
      wrap: [{ wrap: ["break-word", "anywhere", "normal"] }],
      /**
       * Hyphens
       * @see https://tailwindcss.com/docs/hyphens
       */
      hyphens: [{ hyphens: ["none", "manual", "auto"] }],
      /**
       * Content
       * @see https://tailwindcss.com/docs/content
       */
      content: [{ content: ["none", isArbitraryVariable, isArbitraryValue] }],
      // -------------------
      // --- Backgrounds ---
      // -------------------
      /**
       * Background Attachment
       * @see https://tailwindcss.com/docs/background-attachment
       */
      "bg-attachment": [{ bg: ["fixed", "local", "scroll"] }],
      /**
       * Background Clip
       * @see https://tailwindcss.com/docs/background-clip
       */
      "bg-clip": [{ "bg-clip": ["border", "padding", "content", "text"] }],
      /**
       * Background Origin
       * @see https://tailwindcss.com/docs/background-origin
       */
      "bg-origin": [{ "bg-origin": ["border", "padding", "content"] }],
      /**
       * Background Position
       * @see https://tailwindcss.com/docs/background-position
       */
      "bg-position": [{ bg: scaleBgPosition() }],
      /**
       * Background Repeat
       * @see https://tailwindcss.com/docs/background-repeat
       */
      "bg-repeat": [{ bg: scaleBgRepeat() }],
      /**
       * Background Size
       * @see https://tailwindcss.com/docs/background-size
       */
      "bg-size": [{ bg: scaleBgSize() }],
      /**
       * Background Image
       * @see https://tailwindcss.com/docs/background-image
       */
      "bg-image": [
        {
          bg: [
            "none",
            {
              linear: [
                { to: ["t", "tr", "r", "br", "b", "bl", "l", "tl"] },
                isInteger,
                isArbitraryVariable,
                isArbitraryValue
              ],
              radial: ["", isArbitraryVariable, isArbitraryValue],
              conic: [isInteger, isArbitraryVariable, isArbitraryValue]
            },
            isArbitraryVariableImage,
            isArbitraryImage
          ]
        }
      ],
      /**
       * Background Color
       * @see https://tailwindcss.com/docs/background-color
       */
      "bg-color": [{ bg: scaleColor() }],
      /**
       * Gradient Color Stops From Position
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-from-pos": [{ from: scaleGradientStopPosition() }],
      /**
       * Gradient Color Stops Via Position
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-via-pos": [{ via: scaleGradientStopPosition() }],
      /**
       * Gradient Color Stops To Position
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-to-pos": [{ to: scaleGradientStopPosition() }],
      /**
       * Gradient Color Stops From
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-from": [{ from: scaleColor() }],
      /**
       * Gradient Color Stops Via
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-via": [{ via: scaleColor() }],
      /**
       * Gradient Color Stops To
       * @see https://tailwindcss.com/docs/gradient-color-stops
       */
      "gradient-to": [{ to: scaleColor() }],
      // ---------------
      // --- Borders ---
      // ---------------
      /**
       * Border Radius
       * @see https://tailwindcss.com/docs/border-radius
       */
      rounded: [{ rounded: scaleRadius() }],
      /**
       * Border Radius Start
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-s": [{ "rounded-s": scaleRadius() }],
      /**
       * Border Radius End
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-e": [{ "rounded-e": scaleRadius() }],
      /**
       * Border Radius Top
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-t": [{ "rounded-t": scaleRadius() }],
      /**
       * Border Radius Right
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-r": [{ "rounded-r": scaleRadius() }],
      /**
       * Border Radius Bottom
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-b": [{ "rounded-b": scaleRadius() }],
      /**
       * Border Radius Left
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-l": [{ "rounded-l": scaleRadius() }],
      /**
       * Border Radius Start Start
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-ss": [{ "rounded-ss": scaleRadius() }],
      /**
       * Border Radius Start End
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-se": [{ "rounded-se": scaleRadius() }],
      /**
       * Border Radius End End
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-ee": [{ "rounded-ee": scaleRadius() }],
      /**
       * Border Radius End Start
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-es": [{ "rounded-es": scaleRadius() }],
      /**
       * Border Radius Top Left
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-tl": [{ "rounded-tl": scaleRadius() }],
      /**
       * Border Radius Top Right
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-tr": [{ "rounded-tr": scaleRadius() }],
      /**
       * Border Radius Bottom Right
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-br": [{ "rounded-br": scaleRadius() }],
      /**
       * Border Radius Bottom Left
       * @see https://tailwindcss.com/docs/border-radius
       */
      "rounded-bl": [{ "rounded-bl": scaleRadius() }],
      /**
       * Border Width
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w": [{ border: scaleBorderWidth() }],
      /**
       * Border Width Inline
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-x": [{ "border-x": scaleBorderWidth() }],
      /**
       * Border Width Block
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-y": [{ "border-y": scaleBorderWidth() }],
      /**
       * Border Width Inline Start
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-s": [{ "border-s": scaleBorderWidth() }],
      /**
       * Border Width Inline End
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-e": [{ "border-e": scaleBorderWidth() }],
      /**
       * Border Width Block Start
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-bs": [{ "border-bs": scaleBorderWidth() }],
      /**
       * Border Width Block End
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-be": [{ "border-be": scaleBorderWidth() }],
      /**
       * Border Width Top
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-t": [{ "border-t": scaleBorderWidth() }],
      /**
       * Border Width Right
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-r": [{ "border-r": scaleBorderWidth() }],
      /**
       * Border Width Bottom
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-b": [{ "border-b": scaleBorderWidth() }],
      /**
       * Border Width Left
       * @see https://tailwindcss.com/docs/border-width
       */
      "border-w-l": [{ "border-l": scaleBorderWidth() }],
      /**
       * Divide Width X
       * @see https://tailwindcss.com/docs/border-width#between-children
       */
      "divide-x": [{ "divide-x": scaleBorderWidth() }],
      /**
       * Divide Width X Reverse
       * @see https://tailwindcss.com/docs/border-width#between-children
       */
      "divide-x-reverse": ["divide-x-reverse"],
      /**
       * Divide Width Y
       * @see https://tailwindcss.com/docs/border-width#between-children
       */
      "divide-y": [{ "divide-y": scaleBorderWidth() }],
      /**
       * Divide Width Y Reverse
       * @see https://tailwindcss.com/docs/border-width#between-children
       */
      "divide-y-reverse": ["divide-y-reverse"],
      /**
       * Border Style
       * @see https://tailwindcss.com/docs/border-style
       */
      "border-style": [{ border: [...scaleLineStyle(), "hidden", "none"] }],
      /**
       * Divide Style
       * @see https://tailwindcss.com/docs/border-style#setting-the-divider-style
       */
      "divide-style": [{ divide: [...scaleLineStyle(), "hidden", "none"] }],
      /**
       * Border Color
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color": [{ border: scaleColor() }],
      /**
       * Border Color Inline
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-x": [{ "border-x": scaleColor() }],
      /**
       * Border Color Block
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-y": [{ "border-y": scaleColor() }],
      /**
       * Border Color Inline Start
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-s": [{ "border-s": scaleColor() }],
      /**
       * Border Color Inline End
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-e": [{ "border-e": scaleColor() }],
      /**
       * Border Color Block Start
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-bs": [{ "border-bs": scaleColor() }],
      /**
       * Border Color Block End
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-be": [{ "border-be": scaleColor() }],
      /**
       * Border Color Top
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-t": [{ "border-t": scaleColor() }],
      /**
       * Border Color Right
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-r": [{ "border-r": scaleColor() }],
      /**
       * Border Color Bottom
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-b": [{ "border-b": scaleColor() }],
      /**
       * Border Color Left
       * @see https://tailwindcss.com/docs/border-color
       */
      "border-color-l": [{ "border-l": scaleColor() }],
      /**
       * Divide Color
       * @see https://tailwindcss.com/docs/divide-color
       */
      "divide-color": [{ divide: scaleColor() }],
      /**
       * Outline Style
       * @see https://tailwindcss.com/docs/outline-style
       */
      "outline-style": [{ outline: [...scaleLineStyle(), "none", "hidden"] }],
      /**
       * Outline Offset
       * @see https://tailwindcss.com/docs/outline-offset
       */
      "outline-offset": [{ "outline-offset": [isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Outline Width
       * @see https://tailwindcss.com/docs/outline-width
       */
      "outline-w": [{ outline: ["", isNumber, isArbitraryVariableLength, isArbitraryLength] }],
      /**
       * Outline Color
       * @see https://tailwindcss.com/docs/outline-color
       */
      "outline-color": [{ outline: scaleColor() }],
      // ---------------
      // --- Effects ---
      // ---------------
      /**
       * Box Shadow
       * @see https://tailwindcss.com/docs/box-shadow
       */
      shadow: [
        {
          shadow: [
            // Deprecated since Tailwind CSS v4.0.0
            "",
            "none",
            themeShadow,
            isArbitraryVariableShadow,
            isArbitraryShadow
          ]
        }
      ],
      /**
       * Box Shadow Color
       * @see https://tailwindcss.com/docs/box-shadow#setting-the-shadow-color
       */
      "shadow-color": [{ shadow: scaleColor() }],
      /**
       * Inset Box Shadow
       * @see https://tailwindcss.com/docs/box-shadow#adding-an-inset-shadow
       */
      "inset-shadow": [
        {
          "inset-shadow": ["none", themeInsetShadow, isArbitraryVariableShadow, isArbitraryShadow]
        }
      ],
      /**
       * Inset Box Shadow Color
       * @see https://tailwindcss.com/docs/box-shadow#setting-the-inset-shadow-color
       */
      "inset-shadow-color": [{ "inset-shadow": scaleColor() }],
      /**
       * Ring Width
       * @see https://tailwindcss.com/docs/box-shadow#adding-a-ring
       */
      "ring-w": [{ ring: scaleBorderWidth() }],
      /**
       * Ring Width Inset
       * @see https://v3.tailwindcss.com/docs/ring-width#inset-rings
       * @deprecated since Tailwind CSS v4.0.0
       * @see https://github.com/tailwindlabs/tailwindcss/blob/v4.0.0/packages/tailwindcss/src/utilities.ts#L4158
       */
      "ring-w-inset": ["ring-inset"],
      /**
       * Ring Color
       * @see https://tailwindcss.com/docs/box-shadow#setting-the-ring-color
       */
      "ring-color": [{ ring: scaleColor() }],
      /**
       * Ring Offset Width
       * @see https://v3.tailwindcss.com/docs/ring-offset-width
       * @deprecated since Tailwind CSS v4.0.0
       * @see https://github.com/tailwindlabs/tailwindcss/blob/v4.0.0/packages/tailwindcss/src/utilities.ts#L4158
       */
      "ring-offset-w": [{ "ring-offset": [isNumber, isArbitraryLength] }],
      /**
       * Ring Offset Color
       * @see https://v3.tailwindcss.com/docs/ring-offset-color
       * @deprecated since Tailwind CSS v4.0.0
       * @see https://github.com/tailwindlabs/tailwindcss/blob/v4.0.0/packages/tailwindcss/src/utilities.ts#L4158
       */
      "ring-offset-color": [{ "ring-offset": scaleColor() }],
      /**
       * Inset Ring Width
       * @see https://tailwindcss.com/docs/box-shadow#adding-an-inset-ring
       */
      "inset-ring-w": [{ "inset-ring": scaleBorderWidth() }],
      /**
       * Inset Ring Color
       * @see https://tailwindcss.com/docs/box-shadow#setting-the-inset-ring-color
       */
      "inset-ring-color": [{ "inset-ring": scaleColor() }],
      /**
       * Text Shadow
       * @see https://tailwindcss.com/docs/text-shadow
       */
      "text-shadow": [
        {
          "text-shadow": ["none", themeTextShadow, isArbitraryVariableShadow, isArbitraryShadow]
        }
      ],
      /**
       * Text Shadow Color
       * @see https://tailwindcss.com/docs/text-shadow#setting-the-shadow-color
       */
      "text-shadow-color": [{ "text-shadow": scaleColor() }],
      /**
       * Opacity
       * @see https://tailwindcss.com/docs/opacity
       */
      opacity: [{ opacity: [isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Mix Blend Mode
       * @see https://tailwindcss.com/docs/mix-blend-mode
       */
      "mix-blend": [{ "mix-blend": [...scaleBlendMode(), "plus-darker", "plus-lighter"] }],
      /**
       * Background Blend Mode
       * @see https://tailwindcss.com/docs/background-blend-mode
       */
      "bg-blend": [{ "bg-blend": scaleBlendMode() }],
      /**
       * Mask Clip
       * @see https://tailwindcss.com/docs/mask-clip
       */
      "mask-clip": [
        { "mask-clip": ["border", "padding", "content", "fill", "stroke", "view"] },
        "mask-no-clip"
      ],
      /**
       * Mask Composite
       * @see https://tailwindcss.com/docs/mask-composite
       */
      "mask-composite": [{ mask: ["add", "subtract", "intersect", "exclude"] }],
      /**
       * Mask Image
       * @see https://tailwindcss.com/docs/mask-image
       */
      "mask-image-linear-pos": [{ "mask-linear": [isNumber] }],
      "mask-image-linear-from-pos": [{ "mask-linear-from": scaleMaskImagePosition() }],
      "mask-image-linear-to-pos": [{ "mask-linear-to": scaleMaskImagePosition() }],
      "mask-image-linear-from-color": [{ "mask-linear-from": scaleColor() }],
      "mask-image-linear-to-color": [{ "mask-linear-to": scaleColor() }],
      "mask-image-t-from-pos": [{ "mask-t-from": scaleMaskImagePosition() }],
      "mask-image-t-to-pos": [{ "mask-t-to": scaleMaskImagePosition() }],
      "mask-image-t-from-color": [{ "mask-t-from": scaleColor() }],
      "mask-image-t-to-color": [{ "mask-t-to": scaleColor() }],
      "mask-image-r-from-pos": [{ "mask-r-from": scaleMaskImagePosition() }],
      "mask-image-r-to-pos": [{ "mask-r-to": scaleMaskImagePosition() }],
      "mask-image-r-from-color": [{ "mask-r-from": scaleColor() }],
      "mask-image-r-to-color": [{ "mask-r-to": scaleColor() }],
      "mask-image-b-from-pos": [{ "mask-b-from": scaleMaskImagePosition() }],
      "mask-image-b-to-pos": [{ "mask-b-to": scaleMaskImagePosition() }],
      "mask-image-b-from-color": [{ "mask-b-from": scaleColor() }],
      "mask-image-b-to-color": [{ "mask-b-to": scaleColor() }],
      "mask-image-l-from-pos": [{ "mask-l-from": scaleMaskImagePosition() }],
      "mask-image-l-to-pos": [{ "mask-l-to": scaleMaskImagePosition() }],
      "mask-image-l-from-color": [{ "mask-l-from": scaleColor() }],
      "mask-image-l-to-color": [{ "mask-l-to": scaleColor() }],
      "mask-image-x-from-pos": [{ "mask-x-from": scaleMaskImagePosition() }],
      "mask-image-x-to-pos": [{ "mask-x-to": scaleMaskImagePosition() }],
      "mask-image-x-from-color": [{ "mask-x-from": scaleColor() }],
      "mask-image-x-to-color": [{ "mask-x-to": scaleColor() }],
      "mask-image-y-from-pos": [{ "mask-y-from": scaleMaskImagePosition() }],
      "mask-image-y-to-pos": [{ "mask-y-to": scaleMaskImagePosition() }],
      "mask-image-y-from-color": [{ "mask-y-from": scaleColor() }],
      "mask-image-y-to-color": [{ "mask-y-to": scaleColor() }],
      "mask-image-radial": [{ "mask-radial": [isArbitraryVariable, isArbitraryValue] }],
      "mask-image-radial-from-pos": [{ "mask-radial-from": scaleMaskImagePosition() }],
      "mask-image-radial-to-pos": [{ "mask-radial-to": scaleMaskImagePosition() }],
      "mask-image-radial-from-color": [{ "mask-radial-from": scaleColor() }],
      "mask-image-radial-to-color": [{ "mask-radial-to": scaleColor() }],
      "mask-image-radial-shape": [{ "mask-radial": ["circle", "ellipse"] }],
      "mask-image-radial-size": [
        { "mask-radial": [{ closest: ["side", "corner"], farthest: ["side", "corner"] }] }
      ],
      "mask-image-radial-pos": [{ "mask-radial-at": scalePosition() }],
      "mask-image-conic-pos": [{ "mask-conic": [isNumber] }],
      "mask-image-conic-from-pos": [{ "mask-conic-from": scaleMaskImagePosition() }],
      "mask-image-conic-to-pos": [{ "mask-conic-to": scaleMaskImagePosition() }],
      "mask-image-conic-from-color": [{ "mask-conic-from": scaleColor() }],
      "mask-image-conic-to-color": [{ "mask-conic-to": scaleColor() }],
      /**
       * Mask Mode
       * @see https://tailwindcss.com/docs/mask-mode
       */
      "mask-mode": [{ mask: ["alpha", "luminance", "match"] }],
      /**
       * Mask Origin
       * @see https://tailwindcss.com/docs/mask-origin
       */
      "mask-origin": [{ "mask-origin": ["border", "padding", "content", "fill", "stroke", "view"] }],
      /**
       * Mask Position
       * @see https://tailwindcss.com/docs/mask-position
       */
      "mask-position": [{ mask: scaleBgPosition() }],
      /**
       * Mask Repeat
       * @see https://tailwindcss.com/docs/mask-repeat
       */
      "mask-repeat": [{ mask: scaleBgRepeat() }],
      /**
       * Mask Size
       * @see https://tailwindcss.com/docs/mask-size
       */
      "mask-size": [{ mask: scaleBgSize() }],
      /**
       * Mask Type
       * @see https://tailwindcss.com/docs/mask-type
       */
      "mask-type": [{ "mask-type": ["alpha", "luminance"] }],
      /**
       * Mask Image
       * @see https://tailwindcss.com/docs/mask-image
       */
      "mask-image": [{ mask: ["none", isArbitraryVariable, isArbitraryValue] }],
      // ---------------
      // --- Filters ---
      // ---------------
      /**
       * Filter
       * @see https://tailwindcss.com/docs/filter
       */
      filter: [
        {
          filter: [
            // Deprecated since Tailwind CSS v3.0.0
            "",
            "none",
            isArbitraryVariable,
            isArbitraryValue
          ]
        }
      ],
      /**
       * Blur
       * @see https://tailwindcss.com/docs/blur
       */
      blur: [{ blur: scaleBlur() }],
      /**
       * Brightness
       * @see https://tailwindcss.com/docs/brightness
       */
      brightness: [{ brightness: [isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Contrast
       * @see https://tailwindcss.com/docs/contrast
       */
      contrast: [{ contrast: [isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Drop Shadow
       * @see https://tailwindcss.com/docs/drop-shadow
       */
      "drop-shadow": [
        {
          "drop-shadow": [
            // Deprecated since Tailwind CSS v4.0.0
            "",
            "none",
            themeDropShadow,
            isArbitraryVariableShadow,
            isArbitraryShadow
          ]
        }
      ],
      /**
       * Drop Shadow Color
       * @see https://tailwindcss.com/docs/filter-drop-shadow#setting-the-shadow-color
       */
      "drop-shadow-color": [{ "drop-shadow": scaleColor() }],
      /**
       * Grayscale
       * @see https://tailwindcss.com/docs/grayscale
       */
      grayscale: [{ grayscale: ["", isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Hue Rotate
       * @see https://tailwindcss.com/docs/hue-rotate
       */
      "hue-rotate": [{ "hue-rotate": [isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Invert
       * @see https://tailwindcss.com/docs/invert
       */
      invert: [{ invert: ["", isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Saturate
       * @see https://tailwindcss.com/docs/saturate
       */
      saturate: [{ saturate: [isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Sepia
       * @see https://tailwindcss.com/docs/sepia
       */
      sepia: [{ sepia: ["", isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Backdrop Filter
       * @see https://tailwindcss.com/docs/backdrop-filter
       */
      "backdrop-filter": [
        {
          "backdrop-filter": [
            // Deprecated since Tailwind CSS v3.0.0
            "",
            "none",
            isArbitraryVariable,
            isArbitraryValue
          ]
        }
      ],
      /**
       * Backdrop Blur
       * @see https://tailwindcss.com/docs/backdrop-blur
       */
      "backdrop-blur": [{ "backdrop-blur": scaleBlur() }],
      /**
       * Backdrop Brightness
       * @see https://tailwindcss.com/docs/backdrop-brightness
       */
      "backdrop-brightness": [
        { "backdrop-brightness": [isNumber, isArbitraryVariable, isArbitraryValue] }
      ],
      /**
       * Backdrop Contrast
       * @see https://tailwindcss.com/docs/backdrop-contrast
       */
      "backdrop-contrast": [
        { "backdrop-contrast": [isNumber, isArbitraryVariable, isArbitraryValue] }
      ],
      /**
       * Backdrop Grayscale
       * @see https://tailwindcss.com/docs/backdrop-grayscale
       */
      "backdrop-grayscale": [
        { "backdrop-grayscale": ["", isNumber, isArbitraryVariable, isArbitraryValue] }
      ],
      /**
       * Backdrop Hue Rotate
       * @see https://tailwindcss.com/docs/backdrop-hue-rotate
       */
      "backdrop-hue-rotate": [
        { "backdrop-hue-rotate": [isNumber, isArbitraryVariable, isArbitraryValue] }
      ],
      /**
       * Backdrop Invert
       * @see https://tailwindcss.com/docs/backdrop-invert
       */
      "backdrop-invert": [
        { "backdrop-invert": ["", isNumber, isArbitraryVariable, isArbitraryValue] }
      ],
      /**
       * Backdrop Opacity
       * @see https://tailwindcss.com/docs/backdrop-opacity
       */
      "backdrop-opacity": [{ "backdrop-opacity": [isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Backdrop Saturate
       * @see https://tailwindcss.com/docs/backdrop-saturate
       */
      "backdrop-saturate": [
        { "backdrop-saturate": [isNumber, isArbitraryVariable, isArbitraryValue] }
      ],
      /**
       * Backdrop Sepia
       * @see https://tailwindcss.com/docs/backdrop-sepia
       */
      "backdrop-sepia": [{ "backdrop-sepia": ["", isNumber, isArbitraryVariable, isArbitraryValue] }],
      // --------------
      // --- Tables ---
      // --------------
      /**
       * Border Collapse
       * @see https://tailwindcss.com/docs/border-collapse
       */
      "border-collapse": [{ border: ["collapse", "separate"] }],
      /**
       * Border Spacing
       * @see https://tailwindcss.com/docs/border-spacing
       */
      "border-spacing": [{ "border-spacing": scaleUnambiguousSpacing() }],
      /**
       * Border Spacing X
       * @see https://tailwindcss.com/docs/border-spacing
       */
      "border-spacing-x": [{ "border-spacing-x": scaleUnambiguousSpacing() }],
      /**
       * Border Spacing Y
       * @see https://tailwindcss.com/docs/border-spacing
       */
      "border-spacing-y": [{ "border-spacing-y": scaleUnambiguousSpacing() }],
      /**
       * Table Layout
       * @see https://tailwindcss.com/docs/table-layout
       */
      "table-layout": [{ table: ["auto", "fixed"] }],
      /**
       * Caption Side
       * @see https://tailwindcss.com/docs/caption-side
       */
      caption: [{ caption: ["top", "bottom"] }],
      // ---------------------------------
      // --- Transitions and Animation ---
      // ---------------------------------
      /**
       * Transition Property
       * @see https://tailwindcss.com/docs/transition-property
       */
      transition: [
        {
          transition: [
            "",
            "all",
            "colors",
            "opacity",
            "shadow",
            "transform",
            "none",
            isArbitraryVariable,
            isArbitraryValue
          ]
        }
      ],
      /**
       * Transition Behavior
       * @see https://tailwindcss.com/docs/transition-behavior
       */
      "transition-behavior": [{ transition: ["normal", "discrete"] }],
      /**
       * Transition Duration
       * @see https://tailwindcss.com/docs/transition-duration
       */
      duration: [{ duration: [isNumber, "initial", isArbitraryVariable, isArbitraryValue] }],
      /**
       * Transition Timing Function
       * @see https://tailwindcss.com/docs/transition-timing-function
       */
      ease: [{ ease: ["linear", "initial", themeEase, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Transition Delay
       * @see https://tailwindcss.com/docs/transition-delay
       */
      delay: [{ delay: [isNumber, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Animation
       * @see https://tailwindcss.com/docs/animation
       */
      animate: [{ animate: ["none", themeAnimate, isArbitraryVariable, isArbitraryValue] }],
      // ------------------
      // --- Transforms ---
      // ------------------
      /**
       * Backface Visibility
       * @see https://tailwindcss.com/docs/backface-visibility
       */
      backface: [{ backface: ["hidden", "visible"] }],
      /**
       * Perspective
       * @see https://tailwindcss.com/docs/perspective
       */
      perspective: [{ perspective: [themePerspective, isArbitraryVariable, isArbitraryValue] }],
      /**
       * Perspective Origin
       * @see https://tailwindcss.com/docs/perspective-origin
       */
      "perspective-origin": [{ "perspective-origin": scalePositionWithArbitrary() }],
      /**
       * Rotate
       * @see https://tailwindcss.com/docs/rotate
       */
      rotate: [{ rotate: scaleRotate() }],
      /**
       * Rotate X
       * @see https://tailwindcss.com/docs/rotate
       */
      "rotate-x": [{ "rotate-x": scaleRotate() }],
      /**
       * Rotate Y
       * @see https://tailwindcss.com/docs/rotate
       */
      "rotate-y": [{ "rotate-y": scaleRotate() }],
      /**
       * Rotate Z
       * @see https://tailwindcss.com/docs/rotate
       */
      "rotate-z": [{ "rotate-z": scaleRotate() }],
      /**
       * Scale
       * @see https://tailwindcss.com/docs/scale
       */
      scale: [{ scale: scaleScale() }],
      /**
       * Scale X
       * @see https://tailwindcss.com/docs/scale
       */
      "scale-x": [{ "scale-x": scaleScale() }],
      /**
       * Scale Y
       * @see https://tailwindcss.com/docs/scale
       */
      "scale-y": [{ "scale-y": scaleScale() }],
      /**
       * Scale Z
       * @see https://tailwindcss.com/docs/scale
       */
      "scale-z": [{ "scale-z": scaleScale() }],
      /**
       * Scale 3D
       * @see https://tailwindcss.com/docs/scale
       */
      "scale-3d": ["scale-3d"],
      /**
       * Skew
       * @see https://tailwindcss.com/docs/skew
       */
      skew: [{ skew: scaleSkew() }],
      /**
       * Skew X
       * @see https://tailwindcss.com/docs/skew
       */
      "skew-x": [{ "skew-x": scaleSkew() }],
      /**
       * Skew Y
       * @see https://tailwindcss.com/docs/skew
       */
      "skew-y": [{ "skew-y": scaleSkew() }],
      /**
       * Transform
       * @see https://tailwindcss.com/docs/transform
       */
      transform: [{ transform: [isArbitraryVariable, isArbitraryValue, "", "none", "gpu", "cpu"] }],
      /**
       * Transform Origin
       * @see https://tailwindcss.com/docs/transform-origin
       */
      "transform-origin": [{ origin: scalePositionWithArbitrary() }],
      /**
       * Transform Style
       * @see https://tailwindcss.com/docs/transform-style
       */
      "transform-style": [{ transform: ["3d", "flat"] }],
      /**
       * Translate
       * @see https://tailwindcss.com/docs/translate
       */
      translate: [{ translate: scaleTranslate() }],
      /**
       * Translate X
       * @see https://tailwindcss.com/docs/translate
       */
      "translate-x": [{ "translate-x": scaleTranslate() }],
      /**
       * Translate Y
       * @see https://tailwindcss.com/docs/translate
       */
      "translate-y": [{ "translate-y": scaleTranslate() }],
      /**
       * Translate Z
       * @see https://tailwindcss.com/docs/translate
       */
      "translate-z": [{ "translate-z": scaleTranslate() }],
      /**
       * Translate None
       * @see https://tailwindcss.com/docs/translate
       */
      "translate-none": ["translate-none"],
      /**
       * Zoom
       * @see https://tailwindcss.com/docs/zoom
       */
      zoom: [{ zoom: [isInteger, isArbitraryVariable, isArbitraryValue] }],
      // ---------------------
      // --- Interactivity ---
      // ---------------------
      /**
       * Accent Color
       * @see https://tailwindcss.com/docs/accent-color
       */
      accent: [{ accent: scaleColor() }],
      /**
       * Appearance
       * @see https://tailwindcss.com/docs/appearance
       */
      appearance: [{ appearance: ["none", "auto"] }],
      /**
       * Caret Color
       * @see https://tailwindcss.com/docs/just-in-time-mode#caret-color-utilities
       */
      "caret-color": [{ caret: scaleColor() }],
      /**
       * Color Scheme
       * @see https://tailwindcss.com/docs/color-scheme
       */
      "color-scheme": [
        { scheme: ["normal", "dark", "light", "light-dark", "only-dark", "only-light"] }
      ],
      /**
       * Cursor
       * @see https://tailwindcss.com/docs/cursor
       */
      cursor: [
        {
          cursor: [
            "auto",
            "default",
            "pointer",
            "wait",
            "text",
            "move",
            "help",
            "not-allowed",
            "none",
            "context-menu",
            "progress",
            "cell",
            "crosshair",
            "vertical-text",
            "alias",
            "copy",
            "no-drop",
            "grab",
            "grabbing",
            "all-scroll",
            "col-resize",
            "row-resize",
            "n-resize",
            "e-resize",
            "s-resize",
            "w-resize",
            "ne-resize",
            "nw-resize",
            "se-resize",
            "sw-resize",
            "ew-resize",
            "ns-resize",
            "nesw-resize",
            "nwse-resize",
            "zoom-in",
            "zoom-out",
            isArbitraryVariable,
            isArbitraryValue
          ]
        }
      ],
      /**
       * Field Sizing
       * @see https://tailwindcss.com/docs/field-sizing
       */
      "field-sizing": [{ "field-sizing": ["fixed", "content"] }],
      /**
       * Pointer Events
       * @see https://tailwindcss.com/docs/pointer-events
       */
      "pointer-events": [{ "pointer-events": ["auto", "none"] }],
      /**
       * Resize
       * @see https://tailwindcss.com/docs/resize
       */
      resize: [{ resize: ["none", "", "y", "x"] }],
      /**
       * Scroll Behavior
       * @see https://tailwindcss.com/docs/scroll-behavior
       */
      "scroll-behavior": [{ scroll: ["auto", "smooth"] }],
      /**
       * Scrollbar Thumb Color
       * @see https://tailwindcss.com/docs/scrollbar-color
       */
      "scrollbar-thumb-color": [{ "scrollbar-thumb": scaleColor() }],
      /**
       * Scrollbar Track Color
       * @see https://tailwindcss.com/docs/scrollbar-color
       */
      "scrollbar-track-color": [{ "scrollbar-track": scaleColor() }],
      /**
       * Scrollbar Gutter
       * @see https://tailwindcss.com/docs/scrollbar-gutter
       */
      "scrollbar-gutter": [{ "scrollbar-gutter": ["auto", "stable", "both"] }],
      /**
       * Scrollbar Width
       * @see https://tailwindcss.com/docs/scrollbar-width
       */
      "scrollbar-w": [{ scrollbar: ["auto", "thin", "none"] }],
      /**
       * Scroll Margin
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-m": [{ "scroll-m": scaleUnambiguousSpacing() }],
      /**
       * Scroll Margin Inline
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mx": [{ "scroll-mx": scaleUnambiguousSpacing() }],
      /**
       * Scroll Margin Block
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-my": [{ "scroll-my": scaleUnambiguousSpacing() }],
      /**
       * Scroll Margin Inline Start
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-ms": [{ "scroll-ms": scaleUnambiguousSpacing() }],
      /**
       * Scroll Margin Inline End
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-me": [{ "scroll-me": scaleUnambiguousSpacing() }],
      /**
       * Scroll Margin Block Start
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mbs": [{ "scroll-mbs": scaleUnambiguousSpacing() }],
      /**
       * Scroll Margin Block End
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mbe": [{ "scroll-mbe": scaleUnambiguousSpacing() }],
      /**
       * Scroll Margin Top
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mt": [{ "scroll-mt": scaleUnambiguousSpacing() }],
      /**
       * Scroll Margin Right
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mr": [{ "scroll-mr": scaleUnambiguousSpacing() }],
      /**
       * Scroll Margin Bottom
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-mb": [{ "scroll-mb": scaleUnambiguousSpacing() }],
      /**
       * Scroll Margin Left
       * @see https://tailwindcss.com/docs/scroll-margin
       */
      "scroll-ml": [{ "scroll-ml": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-p": [{ "scroll-p": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding Inline
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-px": [{ "scroll-px": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding Block
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-py": [{ "scroll-py": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding Inline Start
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-ps": [{ "scroll-ps": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding Inline End
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pe": [{ "scroll-pe": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding Block Start
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pbs": [{ "scroll-pbs": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding Block End
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pbe": [{ "scroll-pbe": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding Top
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pt": [{ "scroll-pt": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding Right
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pr": [{ "scroll-pr": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding Bottom
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pb": [{ "scroll-pb": scaleUnambiguousSpacing() }],
      /**
       * Scroll Padding Left
       * @see https://tailwindcss.com/docs/scroll-padding
       */
      "scroll-pl": [{ "scroll-pl": scaleUnambiguousSpacing() }],
      /**
       * Scroll Snap Align
       * @see https://tailwindcss.com/docs/scroll-snap-align
       */
      "snap-align": [{ snap: ["start", "end", "center", "align-none"] }],
      /**
       * Scroll Snap Stop
       * @see https://tailwindcss.com/docs/scroll-snap-stop
       */
      "snap-stop": [{ snap: ["normal", "always"] }],
      /**
       * Scroll Snap Type
       * @see https://tailwindcss.com/docs/scroll-snap-type
       */
      "snap-type": [{ snap: ["none", "x", "y", "both"] }],
      /**
       * Scroll Snap Type Strictness
       * @see https://tailwindcss.com/docs/scroll-snap-type
       */
      "snap-strictness": [{ snap: ["mandatory", "proximity"] }],
      /**
       * Touch Action
       * @see https://tailwindcss.com/docs/touch-action
       */
      touch: [{ touch: ["auto", "none", "manipulation"] }],
      /**
       * Touch Action X
       * @see https://tailwindcss.com/docs/touch-action
       */
      "touch-x": [{ "touch-pan": ["x", "left", "right"] }],
      /**
       * Touch Action Y
       * @see https://tailwindcss.com/docs/touch-action
       */
      "touch-y": [{ "touch-pan": ["y", "up", "down"] }],
      /**
       * Touch Action Pinch Zoom
       * @see https://tailwindcss.com/docs/touch-action
       */
      "touch-pz": ["touch-pinch-zoom"],
      /**
       * User Select
       * @see https://tailwindcss.com/docs/user-select
       */
      select: [{ select: ["none", "text", "all", "auto"] }],
      /**
       * Will Change
       * @see https://tailwindcss.com/docs/will-change
       */
      "will-change": [
        {
          "will-change": [
            "auto",
            "scroll",
            "contents",
            "transform",
            isArbitraryVariable,
            isArbitraryValue
          ]
        }
      ],
      // -----------
      // --- SVG ---
      // -----------
      /**
       * Fill
       * @see https://tailwindcss.com/docs/fill
       */
      fill: [{ fill: ["none", ...scaleColor()] }],
      /**
       * Stroke Width
       * @see https://tailwindcss.com/docs/stroke-width
       */
      "stroke-w": [
        {
          stroke: [isNumber, isArbitraryVariableLength, isArbitraryLength, isArbitraryNumber]
        }
      ],
      /**
       * Stroke
       * @see https://tailwindcss.com/docs/stroke
       */
      stroke: [{ stroke: ["none", ...scaleColor()] }],
      // ---------------------
      // --- Accessibility ---
      // ---------------------
      /**
       * Forced Color Adjust
       * @see https://tailwindcss.com/docs/forced-color-adjust
       */
      "forced-color-adjust": [{ "forced-color-adjust": ["auto", "none"] }]
    },
    conflictingClassGroups: {
      "container-named": ["container-type"],
      overflow: ["overflow-x", "overflow-y"],
      overscroll: ["overscroll-x", "overscroll-y"],
      inset: [
        "inset-x",
        "inset-y",
        "inset-bs",
        "inset-be",
        "start",
        "end",
        "top",
        "right",
        "bottom",
        "left"
      ],
      "inset-x": ["right", "left"],
      "inset-y": ["top", "bottom"],
      flex: ["basis", "grow", "shrink"],
      gap: ["gap-x", "gap-y"],
      p: ["px", "py", "ps", "pe", "pbs", "pbe", "pt", "pr", "pb", "pl"],
      px: ["pr", "pl"],
      py: ["pt", "pb"],
      m: ["mx", "my", "ms", "me", "mbs", "mbe", "mt", "mr", "mb", "ml"],
      mx: ["mr", "ml"],
      my: ["mt", "mb"],
      size: ["w", "h"],
      "font-size": ["leading"],
      "fvn-normal": [
        "fvn-ordinal",
        "fvn-slashed-zero",
        "fvn-figure",
        "fvn-spacing",
        "fvn-fraction"
      ],
      "fvn-ordinal": ["fvn-normal"],
      "fvn-slashed-zero": ["fvn-normal"],
      "fvn-figure": ["fvn-normal"],
      "fvn-spacing": ["fvn-normal"],
      "fvn-fraction": ["fvn-normal"],
      "line-clamp": ["display", "overflow"],
      rounded: [
        "rounded-s",
        "rounded-e",
        "rounded-t",
        "rounded-r",
        "rounded-b",
        "rounded-l",
        "rounded-ss",
        "rounded-se",
        "rounded-ee",
        "rounded-es",
        "rounded-tl",
        "rounded-tr",
        "rounded-br",
        "rounded-bl"
      ],
      "rounded-s": ["rounded-ss", "rounded-es"],
      "rounded-e": ["rounded-se", "rounded-ee"],
      "rounded-t": ["rounded-tl", "rounded-tr"],
      "rounded-r": ["rounded-tr", "rounded-br"],
      "rounded-b": ["rounded-br", "rounded-bl"],
      "rounded-l": ["rounded-tl", "rounded-bl"],
      "border-spacing": ["border-spacing-x", "border-spacing-y"],
      "border-w": [
        "border-w-x",
        "border-w-y",
        "border-w-s",
        "border-w-e",
        "border-w-bs",
        "border-w-be",
        "border-w-t",
        "border-w-r",
        "border-w-b",
        "border-w-l"
      ],
      "border-w-x": ["border-w-r", "border-w-l"],
      "border-w-y": ["border-w-t", "border-w-b"],
      "border-color": [
        "border-color-x",
        "border-color-y",
        "border-color-s",
        "border-color-e",
        "border-color-bs",
        "border-color-be",
        "border-color-t",
        "border-color-r",
        "border-color-b",
        "border-color-l"
      ],
      "border-color-x": ["border-color-r", "border-color-l"],
      "border-color-y": ["border-color-t", "border-color-b"],
      translate: ["translate-x", "translate-y", "translate-none"],
      "translate-none": ["translate", "translate-x", "translate-y", "translate-z"],
      "scroll-m": [
        "scroll-mx",
        "scroll-my",
        "scroll-ms",
        "scroll-me",
        "scroll-mbs",
        "scroll-mbe",
        "scroll-mt",
        "scroll-mr",
        "scroll-mb",
        "scroll-ml"
      ],
      "scroll-mx": ["scroll-mr", "scroll-ml"],
      "scroll-my": ["scroll-mt", "scroll-mb"],
      "scroll-p": [
        "scroll-px",
        "scroll-py",
        "scroll-ps",
        "scroll-pe",
        "scroll-pbs",
        "scroll-pbe",
        "scroll-pt",
        "scroll-pr",
        "scroll-pb",
        "scroll-pl"
      ],
      "scroll-px": ["scroll-pr", "scroll-pl"],
      "scroll-py": ["scroll-pt", "scroll-pb"],
      touch: ["touch-x", "touch-y", "touch-pz"],
      "touch-x": ["touch"],
      "touch-y": ["touch"],
      "touch-pz": ["touch"]
    },
    conflictingClassGroupModifiers: {
      "font-size": ["leading"]
    },
    postfixLookupClassGroups: ["container-type"],
    orderSensitiveModifiers: [
      "*",
      "**",
      "after",
      "backdrop",
      "before",
      "details-content",
      "file",
      "first-letter",
      "first-line",
      "marker",
      "placeholder",
      "selection"
    ]
  };
};
var mergeConfigs = (baseConfig, { extend = {}, override = {} }) => {
  overrideConfigProperties(baseConfig.theme, override.theme);
  overrideConfigProperties(baseConfig.classGroups, override.classGroups);
  overrideConfigProperties(baseConfig.conflictingClassGroups, override.conflictingClassGroups);
  overrideConfigProperties(
    baseConfig.conflictingClassGroupModifiers,
    override.conflictingClassGroupModifiers
  );
  overrideProperty(baseConfig, "postfixLookupClassGroups", override.postfixLookupClassGroups);
  overrideProperty(baseConfig, "orderSensitiveModifiers", override.orderSensitiveModifiers);
  mergeConfigProperties(baseConfig.theme, extend.theme);
  mergeConfigProperties(baseConfig.classGroups, extend.classGroups);
  mergeConfigProperties(baseConfig.conflictingClassGroups, extend.conflictingClassGroups);
  mergeConfigProperties(
    baseConfig.conflictingClassGroupModifiers,
    extend.conflictingClassGroupModifiers
  );
  mergeArrayProperties(baseConfig, extend, "postfixLookupClassGroups");
  mergeArrayProperties(baseConfig, extend, "orderSensitiveModifiers");
  return baseConfig;
};
var overrideProperty = (baseObject, overrideKey, overrideValue) => {
  if (overrideValue !== void 0) {
    baseObject[overrideKey] = overrideValue;
  }
};
var overrideConfigProperties = (baseObject, overrideObject) => {
  if (overrideObject) {
    for (const key in overrideObject) {
      overrideProperty(baseObject, key, overrideObject[key]);
    }
  }
};
var mergeConfigProperties = (baseObject, mergeObject) => {
  if (mergeObject) {
    for (const key in mergeObject) {
      mergeArrayProperties(baseObject, mergeObject, key);
    }
  }
};
var mergeArrayProperties = (baseObject, mergeObject, key) => {
  const mergeValue = mergeObject[key];
  if (mergeValue !== void 0) {
    baseObject[key] = baseObject[key] ? baseObject[key].concat(mergeValue) : mergeValue;
  }
};
var createMerger = (config) => {
  if (!config) {
    return createTailwindMerge(getDefaultConfig);
  }
  const createConfig = typeof config === "function" ? () => config(getDefaultConfig()) : () => mergeConfigs(getDefaultConfig(), config);
  return createTailwindMerge(createConfig);
};
var toMergerConfig = (config) => {
  if (isEmptyObject(config)) return void 0;
  const source = config;
  const extend = {
    ...source.extend ?? {}
  };
  for (const key of [
    "theme",
    "classGroups",
    "conflictingClassGroups",
    "conflictingClassGroupModifiers",
    "postfixLookupClassGroups",
    "orderSensitiveModifiers",
    "cacheSize",
    "prefix",
    "separator",
    "experimentalParseClassName"
  ]) {
    if (source[key] !== void 0 && extend[key] === void 0) {
      extend[key] = source[key];
    }
  }
  const result = {};
  if (Object.keys(extend).length > 0) {
    result.extend = extend;
  }
  if (source.override != null && !isEmptyObject(source.override)) {
    result.override = source.override;
  }
  if (!result.extend && !result.override) return void 0;
  return result;
};
var createTwMerge = (cachedTwMergeConfig) => {
  const extension = toMergerConfig(cachedTwMergeConfig);
  const merger = createMerger(extension);
  return (classList) => merger.mergeString(classList);
};
var defaultMerger;
var getDefaultMerger = () => {
  if (!defaultMerger) defaultMerger = createMerger();
  return defaultMerger;
};
var ensureConfiguredMerger = () => {
  if (!state.cachedTwMerge || state.didTwMergeConfigChange) {
    state.didTwMergeConfigChange = false;
    state.cachedTwMerge = createTwMerge(state.cachedTwMergeConfig);
  }
  return state.cachedTwMerge;
};
var syncTwMergeConfig = (config) => {
  const next = config == null ? void 0 : config.twMergeConfig;
  if (!next || isEmptyObject(next)) return;
  if (!isEqual(next, state.cachedTwMergeConfig)) {
    state.cachedTwMergeConfig = next;
    state.didTwMergeConfigChange = true;
  }
};
var joinArgs = (classnames) => joinClassValue(classnames);
var originalStateReset = state.reset.bind(state);
state.reset = () => {
  defaultMerger = void 0;
  originalStateReset();
};
var executeMerge = (classnames, config) => {
  const base = joinArgs(classnames);
  if (!base || !((config == null ? void 0 : config.twMerge) ?? true)) return base || void 0;
  if (base.indexOf(" ") === -1) return base;
  syncTwMergeConfig(config);
  const hasCustomConfig = Boolean((config == null ? void 0 : config.twMergeConfig) && !isEmptyObject(config.twMergeConfig));
  const merge = hasCustomConfig ? ensureConfiguredMerger() : getDefaultMerger().mergeString;
  return merge(base) || void 0;
};
var cnAdapter = (config, ...classnames) => executeMerge(classnames, config);
var runtime = getTailwindVariants(cnAdapter);
var tv = runtime.tv;
const buttonVariants = tv({
  base: "inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-full text-sm font-medium transition-all outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40 active:scale-[0.97] disabled:pointer-events-none disabled:opacity-40 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
  variants: {
    variant: {
      default: "bg-primary text-primary-foreground hover:bg-primary/85",
      secondary: "bg-muted text-foreground hover:bg-accent",
      outline: "border border-border bg-transparent hover:bg-muted",
      ghost: "hover:bg-muted",
      destructive: "bg-destructive text-white hover:bg-destructive/90"
    },
    size: {
      default: "h-9 px-4 max-md:h-11",
      sm: "h-8 px-3 text-xs max-md:h-11 max-md:text-sm",
      lg: "h-11 px-6 text-base",
      icon: "size-9 max-md:size-11",
      "icon-sm": "size-8 max-md:size-11"
    }
  },
  defaultVariants: { variant: "default", size: "default" }
});
function Button($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      class: className = "",
      variant = "default",
      size = "default",
      type = "button",
      children,
      $$slots,
      $$events,
      ...rest
    } = $$props;
    $$renderer2.push(`<button${attributes({
      type,
      class: clsx(cn(buttonVariants({ variant, size }), className)),
      ...rest
    })}>`);
    children == null ? void 0 : children($$renderer2);
    $$renderer2.push(`<!----></button>`);
  });
}
const SETTINGS_GROUPS = [
  {
    title: "Workspace",
    items: [
      { id: "agents", label: "Agents", icon: "bot", hint: "Models, effort and fallbacks per role", scope: "Global" },
      { id: "runs", label: "Runs", icon: "ticket", hint: "Ticket concurrency", scope: "Global" },
      { id: "context", label: "Context & names", icon: "layers", hint: "Compression, Session names", scope: "Global" },
      { id: "memory", label: "Memory", icon: "brain", hint: "Automatic extraction, embeddings, recall weights, deciders and Jev", searchTerms: ["memory_extract_model", "memory_extract_effort", "memory_embedding_model", "memory_recall_weights", "memory_auto_extract", "automatic extraction", "embeddings", "recall weights"], scope: "Admin" },
      { id: "access", label: "Users & access", icon: "shield-check", hint: "Sign-up and account access", scope: "Admin" }
    ]
  },
  {
    title: "You",
    items: [
      { id: "profile", label: "Profile", icon: "user-round", hint: "What agents know about you", scope: "You" },
      { id: "notifications", label: "Notifications", icon: "bell", hint: "Push on this device", scope: "You" },
      { id: "sound-voice", label: "Sound & voice", icon: "volume-2", hint: "Sounds, dictation and install", scope: "You" }
    ]
  }
];
SETTINGS_GROUPS.flatMap((group) => group.items.map((item) => item.id));
Object.fromEntries(SETTINGS_GROUPS.flatMap((group) => group.items.map((item) => [item.id, item])));
function buildPath(route) {
  switch (route == null ? void 0 : route.name) {
    case "home":
      return "/";
    case "project":
      return `/p/${encodeURIComponent(route.projectId)}`;
    case "project-runs":
      return `/p/${encodeURIComponent(route.projectId)}/runs`;
    case "project-settings":
      return `/p/${encodeURIComponent(route.projectId)}/settings`;
    case "project-run":
      return `/p/${encodeURIComponent(route.projectId)}/runs/${encodeURIComponent(route.runId)}`;
    case "project-ticket":
      return `/p/${encodeURIComponent(route.projectId)}/runs/${encodeURIComponent(route.runId)}/${encodeURIComponent(route.ticketId)}`;
    case "session":
      return `/s/${encodeURIComponent(route.id)}${route.at ? `?at=${route.at}` : ""}`;
    case "runs":
      return "/runs";
    case "run":
      return `/runs/${encodeURIComponent(route.runId)}`;
    case "ticket":
      return `/runs/${encodeURIComponent(route.runId)}/${encodeURIComponent(route.ticketId)}`;
    case "settings":
      return route.section ? `/settings/${encodeURIComponent(route.section)}` : "/settings";
    case "quota":
      return "/quota";
    case "providers":
      return "/providers";
    case "memory":
      return "/memory";
    default:
      return "/";
  }
}
function DashboardPage($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    var _a;
    let {
      projects = [],
      onNavigate = () => {
      },
      initialDashboard = null
    } = $$props;
    const bootDashboard = typeof window !== "undefined" ? (_a = window.__EL_BOOT__) == null ? void 0 : _a["/api/dashboard"] : null;
    const initialPayload = initialDashboard ?? bootDashboard;
    let groups = (initialPayload == null ? void 0 : initialPayload.groups) ?? {};
    let loaded = Boolean(initialPayload);
    const projectNames = derived(() => new Map(projects.map((project) => [project.id, project.name])));
    const activeGroups = derived(() => Object.entries(groups).map(([projectId, group]) => ({
      projectId,
      name: projectId === "none" ? "No project" : projectNames().get(projectId) ?? "Project not found",
      sessions: (group.sessions ?? []).filter((session) => session.status === "working" || session.status === "needs-you"),
      runs: (group.runs ?? []).filter((run) => run.status === "running" || run.status === "needs-you")
    })).filter((group) => group.sessions.length || group.runs.length).sort((a, b) => a.name.localeCompare(b.name)));
    $$renderer2.push(`<div class="min-h-0 flex-1 overflow-auto" data-testid="dashboard"><div class="mx-auto flex w-full max-w-5xl flex-col gap-5 px-4 py-6 sm:px-6"><header class="animate-rise"><p class="text-xs font-medium uppercase tracking-wide text-muted-foreground">Workspace</p> <h1 class="mt-1 text-2xl font-semibold tracking-tight">All projects</h1> <p class="mt-1 text-sm text-muted-foreground">Work that is running or needs your attention.</p></header> `);
    {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--> `);
    if (!loaded) {
      $$renderer2.push(`<!--[0--><p role="status" class="rounded-xl border px-4 py-3 text-sm text-muted-foreground">Loading Dashboard…</p>`);
    } else if (activeGroups().length) {
      $$renderer2.push(`<!--[1--><div class="grid gap-4"><!--[-->`);
      const each_array = ensure_array_like(activeGroups());
      for (let $$index_2 = 0, $$length = each_array.length; $$index_2 < $$length; $$index_2++) {
        let group = each_array[$$index_2];
        $$renderer2.push(`<section class="animate-rise rounded-2xl border bg-card p-4"${attr("aria-label", group.name)} data-testid="dashboard-project"><div class="mb-3 flex items-center justify-between gap-3"><h2 class="truncate text-base font-semibold">${escape_html(group.name)}</h2> `);
        if (group.projectId !== "none") {
          $$renderer2.push("<!--[0-->");
          Button($$renderer2, {
            variant: "ghost",
            size: "sm",
            onclick: () => onNavigate(buildPath({ name: "project", projectId: group.projectId })),
            children: ($$renderer3) => {
              $$renderer3.push(`<!---->Open project`);
            },
            $$slots: { default: true }
          });
        } else {
          $$renderer2.push("<!--[-1-->");
          Button($$renderer2, {
            variant: "ghost",
            size: "sm",
            onclick: () => onNavigate(buildPath({ name: "project", projectId: "none" })),
            children: ($$renderer3) => {
              $$renderer3.push(`<!---->Open no-project Sessions`);
            },
            $$slots: { default: true }
          });
        }
        $$renderer2.push(`<!--]--></div> `);
        if (group.sessions.length) {
          $$renderer2.push(`<!--[0--><div class="mb-3"><h3 class="mb-1 px-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Sessions</h3> <ul class="flex flex-col gap-1"><!--[-->`);
          const each_array_1 = ensure_array_like(group.sessions);
          for (let $$index = 0, $$length2 = each_array_1.length; $$index < $$length2; $$index++) {
            let session = each_array_1[$$index];
            $$renderer2.push(`<li>`);
            Button($$renderer2, {
              variant: "ghost",
              class: "h-auto min-h-11 w-full justify-start text-left",
              "data-testid": "dashboard-session",
              onclick: () => onNavigate(buildPath({ name: "session", id: session.id })),
              children: ($$renderer3) => {
                $$renderer3.push(`<span class="min-w-0 flex-1 truncate">${escape_html(session.title || session.id)}</span> <span class="shrink-0 text-xs text-muted-foreground">${escape_html(session.status === "working" ? "Running" : "Needs you")}</span>`);
              },
              $$slots: { default: true }
            });
            $$renderer2.push(`<!----></li>`);
          }
          $$renderer2.push(`<!--]--></ul></div>`);
        } else {
          $$renderer2.push("<!--[-1-->");
        }
        $$renderer2.push(`<!--]--> `);
        if (group.runs.length) {
          $$renderer2.push(`<!--[0--><div><h3 class="mb-1 px-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Runs</h3> <ul class="flex flex-col gap-1"><!--[-->`);
          const each_array_2 = ensure_array_like(group.runs);
          for (let $$index_1 = 0, $$length2 = each_array_2.length; $$index_1 < $$length2; $$index_1++) {
            let run = each_array_2[$$index_1];
            $$renderer2.push(`<li>`);
            Button($$renderer2, {
              variant: "ghost",
              class: "h-auto min-h-11 w-full justify-start text-left",
              "data-testid": "dashboard-run",
              onclick: () => onNavigate(buildPath({
                name: "project-run",
                projectId: group.projectId,
                runId: run.id
              })),
              children: ($$renderer3) => {
                $$renderer3.push(`<span class="min-w-0 flex-1 truncate">${escape_html(run.spec || run.id)}</span> <span class="shrink-0 text-xs text-muted-foreground">${escape_html(run.status === "running" ? "Running" : "Needs you")}</span>`);
              },
              $$slots: { default: true }
            });
            $$renderer2.push(`<!----></li>`);
          }
          $$renderer2.push(`<!--]--></ul></div>`);
        } else {
          $$renderer2.push("<!--[-1-->");
        }
        $$renderer2.push(`<!--]--></section>`);
      }
      $$renderer2.push(`<!--]--></div>`);
    } else {
      $$renderer2.push(`<!--[-1--><p class="rounded-2xl border px-4 py-8 text-center text-sm text-muted-foreground">Nothing needs attention. Running and needs-you work will appear here.</p>`);
    }
    $$renderer2.push(`<!--]--></div></div>`);
  });
}
function renderDashboard(projectsJSON = "[]", dashboardJSON = "{}") {
  const projects = typeof projectsJSON === "string" ? JSON.parse(projectsJSON || "[]") : projectsJSON;
  const dashboard = typeof dashboardJSON === "string" ? JSON.parse(dashboardJSON || "{}") : dashboardJSON;
  return render(DashboardPage, { props: { projects, initialDashboard: dashboard } }).body;
}
globalThis.__elRenderDashboard = renderDashboard;
export {
  renderDashboard
};
