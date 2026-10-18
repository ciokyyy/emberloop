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
var __privateWrapper = (obj, member, setter, getter) => ({
  set _(value) {
    __privateSet(obj, member, value, setter);
  },
  get _() {
    return __privateGet(obj, member, getter);
  }
});
var _render, _render_async, _sync, _hashes, _promise, _RenderResult_instances, get_fn, _out, _on_destroy, _is_component_body, _boundary, _parent, _Renderer_static, serialize_failed_boundary_fn, _Renderer_instances, collect_on_destroy_fn, traverse_components_fn, collect_ondestroy_fn, run_on_destroy_fn, create_fn, render_fn, render_async_fn, collect_content_fn, collect_content_async_fn, collect_hydratables_fn, open_render_fn, close_render_fn, hydratable_block_fn, _pending, _controller, _aborted, _title, _document, _subscribe, _name, _key, _size, _observed, _options, _node, _window, _width, _height, _subscribe2, _root, _variant, _prefix, _opts, _currentFrame, _observer, _runId, _AnimationsComplete_instances, cleanup_fn, executeCallback_fn, _opts2, _enabled, _afterAnimations, _shouldRender, _transitionStatus, _hasMounted, _transitionFrame, _PresenceManager_instances, clearTransitionFrame_fn, _interactOutsideProp, _behaviorType, _interceptedEvents, _isResponsibleLayer, _capturedPointerDown, _isFocusInsideDOMTree, _documentObj, _onFocusOutside, _unsubClickListener, _destroyed, _handleFocus, _DismissibleLayerState_instances, addEventListeners_fn, _handleDismiss, _interactOutside, _handleInteractOutside, _markInterceptedEvent, _markNonInterceptedEvent, _markResponsibleLayer, _isTargetWithinLayer, _resetState, isAnyEventIntercepted_fn, _onfocuscapture, _onblurcapture, _addEventListener, _onkeydown, _scopeStack, _focusHistory, _preFocusHistory, _paused, _container, _manager, _cleanupFns, _opts3, _FocusScope_instances, cleanup_fn2, handleOpenAutoFocus_fn, handleCloseAutoFocus_fn, setupEventListeners_fn, getTabbables_fn, getFirstTabbable_fn, getAllFocusables_fn, _unsubSelectionLock, _enabledSnapshot, _onPointerDownSnapshot, _onPointerUpSnapshot, _TextSelectionLayerState_instances, addEventListeners_fn2, _pointerup, _pointerdown, _resetSelectionLock, _factory, _subscribers, _state, _scope, _SharedState_instances, dispose_fn, _id, _initialState, _restoreScrollDelay, _countState, _transformedStyle, _updatePositionStrategy, _arrowSize, _arrowWidth, _arrowHeight, _desiredPlacement, _boundary2, _hasExplicitBoundaries, _detectOverflowOptions, _availableWidth, _availableHeight, _anchorWidth, _anchorHeight, _middleware, _placedSide, _placedAlign, _arrowX, _arrowY, _cannotCenterArrow, _arrowBaseSide, _wrapperProps, _props, _arrowStyle, _opts4, _buffer, _transitIntentTimeout, _exitPoint, _exitTarget, _transitTargets, _trackedTriggerNode, _leaveFallbackRafId, _transitIntentTimeoutId, _SafePolygon_instances, cancelLeaveFallback_fn, scheduleLeaveFallback_fn, cancelTransitIntentTimeout_fn, scheduleTransitIntentTimeout_fn, onPointerMove_fn, clearTracking_fn, getCorridorPolygon_fn, getSafePolygon_fn, flipSide_fn, _closeTimeout, _domContext, _PopoverRootState_instances, clearCloseTimeout_fn, _openTimeout, _closeTimeout2, _isHovering, _PopoverTriggerState_instances, clearOpenTimeout_fn, clearCloseTimeout_fn2, clearAllTimeouts_fn, getAriaControls_fn, _props2, _snippetProps, _props3, _interval, _cb, _timer, _TimeoutFn_instances, clear_fn, _activeTriggerNode, _activePayload, _coerceActiveTrigger, _timerFn, _openTooltip, _startTimer, _clearTimer, _delayDuration, _disableHoverableContent, _disableCloseOnTriggerClick, _disabled, _ignoreNonKeyboardFocus, _wasOpenDelayed, _timerFn2, _stateAttr, _handleDelayedOpen, _isPointerDown, _hasPointerMoveOpened, _transitCheckTimeout, _mounted, _lastRegisteredId, _getRoot, _isDisabled, _register, _clearTransitCheck, _onpointerup, _onpointerdown, _onpointerenter, _onpointermove, _onpointerleave, _onfocus, _onblur, _onclick, _props4, _snippetProps2, _props5;
const HYDRATION_START = "[";
const HYDRATION_START_FAILED = "[?";
const HYDRATION_END = "]";
const ELEMENT_IS_NAMESPACED = 1;
const ELEMENT_PRESERVE_ATTRIBUTE_CASE = 1 << 1;
const ELEMENT_IS_INPUT = 1 << 2;
const UNINITIALIZED = Symbol("uninitialized");
const ATTACHMENT_KEY = "@attach";
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
var define_property = Object.defineProperty;
var has_own_property = Object.prototype.hasOwnProperty;
const noop$1 = () => {
};
function run(fn) {
  return fn();
}
function run_all(arr) {
  for (var i = 0; i < arr.length; i++) {
    arr[i]();
  }
}
function deferred() {
  var resolve;
  var reject;
  var promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}
function fallback(value, fallback2, lazy = false) {
  return value === void 0 ? lazy ? (
    /** @type {() => V} */
    fallback2()
  ) : (
    /** @type {V} */
    fallback2
  ) : value;
}
function to_array(value, n) {
  if (Array.isArray(value)) {
    return value;
  }
  if (!(Symbol.iterator in value)) {
    return Array.from(value);
  }
  const array = [];
  for (const element2 of value) {
    array.push(element2);
    if (array.length === n) break;
  }
  return array;
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
function lifecycle_outside_component(name) {
  {
    throw new Error(`https://svelte.dev/e/lifecycle_outside_component`);
  }
}
function set_context_after_init() {
  {
    throw new Error(`https://svelte.dev/e/set_context_after_init`);
  }
}
function get_parent_context(context2) {
  let parent = context2.p;
  while (parent !== null && parent.c === null) {
    parent = parent.p;
  }
  return (parent == null ? void 0 : parent.c) ?? null;
}
function get_or_init_context_map(context2, name) {
  if (context2 === null) {
    lifecycle_outside_component();
  }
  return context2.c ?? (context2.c = new Map(get_parent_context(context2) || void 0));
}
let micro_tasks = [];
function run_micro_tasks() {
  var tasks = micro_tasks;
  micro_tasks = [];
  run_all(tasks);
}
function queue_micro_task(fn) {
  if (micro_tasks.length === 0 && true) {
    var tasks = micro_tasks;
    queueMicrotask(() => {
      if (tasks === micro_tasks) run_micro_tasks();
    });
  }
  micro_tasks.push(fn);
}
function without_reactive_context(fn) {
  try {
    return fn();
  } finally {
  }
}
const BLOCK_OPEN = `<!--${HYDRATION_START}-->`;
const BLOCK_CLOSE = `<!--${HYDRATION_END}-->`;
const EMPTY_COMMENT = `<!---->`;
const VOID_ELEMENT_NAMES = [
  "area",
  "base",
  "br",
  "col",
  "command",
  "embed",
  "hr",
  "img",
  "input",
  "keygen",
  "link",
  "meta",
  "param",
  "source",
  "track",
  "wbr"
];
function is_void(name) {
  return VOID_ELEMENT_NAMES.includes(name) || name.toLowerCase() === "!doctype";
}
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
const RAW_TEXT_ELEMENTS = (
  /** @type {const} */
  ["textarea", "script", "style", "title"]
);
function is_raw_text_element(name) {
  return RAW_TEXT_ELEMENTS.includes(
    /** @type {typeof RAW_TEXT_ELEMENTS[number]} */
    name
  );
}
const REGEX_VALID_TAG_NAME = /^[a-zA-Z][a-zA-Z0-9]*(-[a-zA-Z0-9.\-_\u00B7\u00C0-\u00D6\u00D8-\u00F6\u00F8-\u037D\u037F-\u1FFF\u200C-\u200D\u203F-\u2040\u2070-\u218F\u2C00-\u2FEF\u3001-\uD7FF\uF900-\uFDCF\uFDF0-\uFFFD\u{10000}-\u{EFFFF}]*)?$/u;
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
function dynamic_element_invalid_tag(tag) {
  const error = new Error(`dynamic_element_invalid_tag
\`<svelte:element this="${tag}">\` is not a valid element name — the element will not be rendered
https://svelte.dev/e/dynamic_element_invalid_tag`);
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
function lifecycle_function_unavailable(name) {
  const error = new Error(`lifecycle_function_unavailable
\`${name}(...)\` is not available on the server
https://svelte.dev/e/lifecycle_function_unavailable`);
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
function getContext(key) {
  const context_map = get_or_init_context_map(ssr_context);
  const result = (
    /** @type {T} */
    context_map.get(key)
  );
  return result;
}
function setContext(key, context2) {
  const context_map = get_or_init_context_map(ssr_context);
  if (
    /** @type {SSRContext} */
    ssr_context.i
  ) {
    set_context_after_init();
  }
  context_map.set(key, context2);
  return context2;
}
function hasContext(key) {
  return get_or_init_context_map(ssr_context).has(key);
}
function getAllContexts() {
  return get_or_init_context_map(ssr_context);
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
  }).then(noop$1, noop$1));
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
  function stringify2(thing) {
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
        return `Object(${stringify2(thing.valueOf())})`;
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
            result += stringify2(thing[i]);
          } else if (!has_holes) {
            const populated_keys = valid_array_indices(thing);
            const population = populated_keys.length;
            const d = String(thing.length).length;
            const array = stringify_sparse_array(thing.length);
            const hole_cost = thing.length + 2;
            const sparse_cost = array.length + 18 + population * (d + 2);
            if (hole_cost > sparse_cost) {
              const entries = populated_keys.map((k) => `${k}:${stringify2(thing[k])}`).join(",");
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
        return `new ${type}([${Array.from(thing).map(stringify2).join(",")}])`;
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
          str2 += `(${stringify2(thing.buffer)})`;
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
          str2 += `(${stringify2(thing.buffer)}`;
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
        const obj = keys2.map((key) => `${safe_key(key)}:${stringify2(thing[key])}`).join(",");
        const proto = Object.getPrototypeOf(thing);
        if (proto === null) {
          return keys2.length > 0 ? `{${obj},__proto__:null}` : `{__proto__:null}`;
        }
        return `{${obj}}`;
    }
  }
  const str = stringify2(value);
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
            reconstructions.push(`${name}=Object(${stringify2(primitive)})`);
          } else {
            values.push(`Object(${stringify2(primitive)})`);
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
            statements.push(`${name}[${i}]=${stringify2(thing[i])}`);
          }
          break;
        }
        case "Set": {
          values.push(`new Set`);
          const adds = Array.from(thing).map((v) => `.add(${stringify2(v)})`);
          if (adds.length > 0) statements.push(name + adds.join(""));
          break;
        }
        case "Map": {
          values.push(`new Map`);
          const sets = Array.from(thing).map(
            ([k, v]) => `.set(${stringify2(k)}, ${stringify2(v)})`
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
            str2 += `(${stringify2(thing.buffer)})`;
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
            str2 += `(${stringify2(thing.buffer)}`;
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
            statements.push(`${name}${safe_prop(key)}=${stringify2(thing[key])}`);
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
    return Array.from(array, (element2) => `${element2}n`).join(",");
  }
  if (array instanceof Float32Array || array instanceof Float64Array || typeof Float16Array !== "undefined" && array instanceof Float16Array) {
    return Array.from(array, (element2) => Object.is(element2, -0) ? "-0" : `${element2}`).join(",");
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
    promise.catch(noop$1);
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
      result.catch(noop$1);
      result.finally(() => set_ssr_context(null)).catch(noop$1);
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
        result.catch(noop$1);
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
            failed_snippet(child, transformed, noop$1);
            __privateGet(child, _out).push(BLOCK_CLOSE);
          })
        );
        child.promise.catch(noop$1);
      } else {
        __privateGet(child, _out).push(__privateMethod(_a = _Renderer, _Renderer_static, serialize_failed_boundary_fn).call(_a, result));
        failed_snippet(child, result, noop$1);
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
          failed(failed_renderer, transformed, noop$1);
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
function element(renderer, tag, attributes_fn = noop$1, children_fn = noop$1) {
  renderer.push("<!---->");
  if (tag) {
    if (!REGEX_VALID_TAG_NAME.test(tag)) {
      dynamic_element_invalid_tag(tag);
    }
    renderer.push(`<${tag}`);
    attributes_fn();
    renderer.push(`>`);
    if (!is_void(tag)) {
      children_fn();
      if (!is_raw_text_element(tag)) {
        renderer.push(EMPTY_COMMENT);
      }
      renderer.push(`</${tag}>`);
    }
  }
  renderer.push("<!---->");
}
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
function spread_props(props) {
  const merged_props = {};
  let key;
  for (let i = 0; i < props.length; i++) {
    const obj = props[i];
    if (obj == null) continue;
    for (key of Object.keys(obj)) {
      const desc = Object.getOwnPropertyDescriptor(obj, key);
      if (desc) {
        Object.defineProperty(merged_props, key, desc);
      } else {
        merged_props[key] = obj[key];
      }
    }
  }
  return merged_props;
}
function stringify(value) {
  return typeof value === "string" ? value : value == null ? "" : value + "";
}
function attr_class(value, hash, directives) {
  var result = to_class(value, hash, directives);
  return result ? ` class="${escape_html(result, true)}"` : "";
}
function bind_props(props_parent, props_now) {
  var _a;
  for (const key of Object.keys(props_now)) {
    const initial_value = props_parent[key];
    const value = props_now[key];
    if (initial_value === void 0 && value !== void 0 && ((_a = Object.getOwnPropertyDescriptor(props_parent, key)) == null ? void 0 : _a.set)) {
      props_parent[key] = value;
    }
  }
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
function props_id(renderer) {
  const uid = renderer.global.uid();
  renderer.push("<!--$" + uid + "-->");
  return uid;
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
function mount() {
  lifecycle_function_unavailable("mount");
}
function unmount() {
  lifecycle_function_unavailable("unmount");
}
async function tick() {
}
/**
 * @file
 * @license @lucide/svelte v1.53.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */
const defaultAttributes = {
  xmlns: "http://www.w3.org/2000/svg",
  width: 24,
  height: 24,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  "stroke-width": 2,
  "stroke-linecap": "round",
  "stroke-linejoin": "round"
};
/**
 * @file
 * @license @lucide/svelte v1.53.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */
const mergeClasses = (...classes) => classes.filter((className, index2, array) => {
  return Boolean(className) && className.trim() !== "" && array.indexOf(className) === index2;
}).join(" ").trim();
/**
 * @file
 * @license @lucide/svelte v1.53.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */
function isDefined(value) {
  return value !== null && value !== void 0;
}
function buildLucideIconNode(icon, params = {}) {
  var _a, _b;
  const attributeNames = params.attributeNames ?? {};
  const getAttributeName = (attributeName) => attributeNames[attributeName] ?? attributeName;
  const viewBoxWidth = icon.size ?? icon.width ?? defaultAttributes["width"];
  const viewBoxHeight = icon.size ?? icon.height ?? defaultAttributes["height"];
  const aliasClassNames = ((_a = icon.aliases) == null ? void 0 : _a.filter((alias) => typeof alias === "string" && alias.trim() !== "").map((alias) => `lucide-${alias}`)) ?? [];
  const iconClassNames = [...icon.name ? [`lucide-${icon.name}`] : [], ...aliasClassNames];
  const classNamesFromClassName = ((_b = params.className) == null ? void 0 : _b.split(" ").filter(Boolean)) ?? [];
  const className = params.includeDefaultClasses === false ? mergeClasses(...classNamesFromClassName) : mergeClasses("lucide", ...iconClassNames, ...classNamesFromClassName);
  const calculatedStrokeWidth = params.absoluteStrokeWidth ? Number(params.strokeWidth ?? defaultAttributes["stroke-width"]) * Number(icon.size ?? icon.width ?? defaultAttributes["width"]) / Number(params.size ?? params.width ?? defaultAttributes["width"]) : params.strokeWidth ?? defaultAttributes["stroke-width"];
  const attributes2 = {
    ...Object.entries(defaultAttributes).reduce((attrs, [attrName, value]) => {
      attrs[getAttributeName(attrName)] = value;
      return attrs;
    }, {}),
    ..."color" in params && params.color && {
      [getAttributeName("stroke")]: params.color
    },
    ..."size" in params && isDefined(params.size) && {
      [getAttributeName("width")]: params.size,
      [getAttributeName("height")]: params.size
    },
    ..."width" in params && isDefined(params.width) && {
      [getAttributeName("width")]: params.width
    },
    ..."height" in params && isDefined(params.height) && {
      [getAttributeName("height")]: params.height
    },
    [getAttributeName("stroke-width")]: calculatedStrokeWidth,
    ...className && {
      [getAttributeName("class")]: className
    },
    [getAttributeName("viewBox")]: `0 0 ${viewBoxWidth} ${viewBoxHeight}`,
    ...params.hasA11yProp === false ? {
      [getAttributeName("aria-hidden")]: "true"
    } : {},
    ..."attributes" in params && params.attributes
  };
  return [
    "svg",
    attributes2,
    icon.node.map((child) => {
      const [name, attrs, children] = child;
      const nextAttrs = params.nonScalingStroke ? { [getAttributeName("vector-effect")]: "non-scaling-stroke", ...attrs } : attrs;
      return children ? [name, nextAttrs, children] : [name, nextAttrs];
    })
  ];
}
/**
 * @file
 * @license @lucide/svelte v1.53.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */
const hasA11yProp = (props) => {
  for (const prop in props) {
    if (prop.startsWith("aria-") || prop === "role" || prop === "title") {
      return true;
    }
  }
  return false;
};
/**
 * @file
 * @license @lucide/svelte v1.53.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */
const LucideContext = Symbol("lucide-context");
const getLucideContext = () => getContext(LucideContext);
function Icon($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    const globalProps = getLucideContext() ?? {};
    const {
      color = globalProps.color ?? "currentColor",
      size: size2 = globalProps.size ?? 24,
      width = size2,
      height = size2,
      strokeWidth = globalProps.strokeWidth ?? 2,
      absoluteStrokeWidth = globalProps.absoluteStrokeWidth ?? false,
      nonScalingStroke = globalProps.nonScalingStroke ?? false,
      iconNode = [],
      icon = { node: iconNode, aliases: [], size: 24 },
      class: propsClass,
      children,
      $$slots,
      $$events,
      ...props
    } = $$props;
    const hasAccessibleProp = derived(() => Boolean(children) || hasA11yProp(props));
    const $$d = derived(() => buildLucideIconNode(icon, {
      color,
      width,
      height,
      strokeWidth,
      absoluteStrokeWidth,
      nonScalingStroke,
      // @TODO: maybe drop the extra `lucide-icon` class altogether.
      className: mergeClasses("lucide-icon", globalProps.class),
      hasA11yProp: hasAccessibleProp(),
      attributes: props
    })), $$derived_array = derived(() => to_array($$d(), 3)), svgAttributes = derived(() => $$derived_array()[1]), builtIconNode = derived(() => fallback($$derived_array()[2], () => [], true));
    const iconAttributes = derived(() => ({
      ...svgAttributes(),
      class: [...svgAttributes().class.split(" "), propsClass]
    }));
    $$renderer2.push(`<svg${attributes({ ...iconAttributes() }, void 0, void 0, void 0, 3)}><!--[-->`);
    const each_array = ensure_array_like(builtIconNode());
    for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
      let [tag, attrs] = each_array[$$index];
      element($$renderer2, tag, () => {
        $$renderer2.push(`${attributes({ ...attrs }, void 0, void 0, void 0, 3)}`);
      });
    }
    $$renderer2.push(`<!--]-->`);
    children == null ? void 0 : children($$renderer2);
    $$renderer2.push(`<!----></svg>`);
  });
}
function Arrow_left($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "arrow-left",
    "size": 24,
    "node": [
      ["path", { "d": "m12 19-7-7 7-7" }],
      ["path", { "d": "M19 12H5" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Ban($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "ban",
    "size": 24,
    "node": [
      ["circle", { "cx": "12", "cy": "12", "r": "10" }],
      ["path", { "d": "M4.929 4.929 19.07 19.071" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Bot($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "bot",
    "size": 24,
    "node": [
      ["path", { "d": "M12 8V4H8" }],
      [
        "rect",
        { "width": "16", "height": "12", "x": "4", "y": "8", "rx": "2" }
      ],
      ["path", { "d": "M2 14h2" }],
      ["path", { "d": "M20 14h2" }],
      ["path", { "d": "M15 13v2" }],
      ["path", { "d": "M9 13v2" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Brain($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "brain",
    "size": 24,
    "node": [
      ["path", { "d": "M12 18V5" }],
      [
        "path",
        { "d": "M15 13a4.17 4.17 0 0 1-3-4 4.17 4.17 0 0 1-3 4" }
      ],
      [
        "path",
        { "d": "M17.598 6.5A3 3 0 1 0 12 5a3 3 0 1 0-5.598 1.5" }
      ],
      ["path", { "d": "M17.997 5.125a4 4 0 0 1 2.526 5.77" }],
      ["path", { "d": "M18 18a4 4 0 0 0 2-7.464" }],
      [
        "path",
        { "d": "M19.967 17.483A4 4 0 1 1 12 18a4 4 0 1 1-7.967-.517" }
      ],
      ["path", { "d": "M6 18a4 4 0 0 1-2-7.464" }],
      ["path", { "d": "M6.003 5.125a4 4 0 0 0-2.526 5.77" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Chevron_down($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "chevron-down",
    "size": 24,
    "node": [["path", { "d": "m6 9 6 6 6-6" }]]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Chevron_right($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "chevron-right",
    "size": 24,
    "node": [["path", { "d": "m9 18 6-6-6-6" }]]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Circle_alert($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "circle-alert",
    "size": 24,
    "node": [
      ["circle", { "cx": "12", "cy": "12", "r": "10" }],
      ["line", { "x1": "12", "x2": "12", "y1": "8", "y2": "12" }],
      [
        "line",
        { "x1": "12", "x2": "12.01", "y1": "16", "y2": "16" }
      ]
    ],
    "aliases": ["alert-circle"]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Circle_check($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "circle-check",
    "size": 24,
    "node": [
      ["circle", { "cx": "12", "cy": "12", "r": "10" }],
      ["path", { "d": "m16 9-5.5 5.5L8 12" }]
    ],
    "aliases": ["check-circle-2"]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Circle_pause($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "circle-pause",
    "size": 24,
    "node": [
      ["circle", { "cx": "12", "cy": "12", "r": "10" }],
      ["line", { "x1": "10", "x2": "10", "y1": "15", "y2": "9" }],
      ["line", { "x1": "14", "x2": "14", "y1": "15", "y2": "9" }]
    ],
    "aliases": ["pause-circle"]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Circle_x($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "circle-x",
    "size": 24,
    "node": [
      ["circle", { "cx": "12", "cy": "12", "r": "10" }],
      ["path", { "d": "m15 9-6 6" }],
      ["path", { "d": "m9 9 6 6" }]
    ],
    "aliases": ["x-circle"]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Circle($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "circle",
    "size": 24,
    "node": [["circle", { "cx": "12", "cy": "12", "r": "10" }]]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Copy($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "copy",
    "size": 24,
    "node": [
      [
        "rect",
        {
          "width": "14",
          "height": "14",
          "x": "8",
          "y": "8",
          "rx": "2",
          "ry": "2"
        }
      ],
      [
        "path",
        {
          "d": "M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"
        }
      ]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Cpu($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "cpu",
    "size": 24,
    "node": [
      ["path", { "d": "M12 20v2" }],
      ["path", { "d": "M12 2v2" }],
      ["path", { "d": "M17 20v2" }],
      ["path", { "d": "M17 2v2" }],
      ["path", { "d": "M2 12h2" }],
      ["path", { "d": "M2 17h2" }],
      ["path", { "d": "M2 7h2" }],
      ["path", { "d": "M20 12h2" }],
      ["path", { "d": "M20 17h2" }],
      ["path", { "d": "M20 7h2" }],
      ["path", { "d": "M7 20v2" }],
      ["path", { "d": "M7 2v2" }],
      [
        "rect",
        { "x": "4", "y": "4", "width": "16", "height": "16", "rx": "2" }
      ],
      [
        "rect",
        { "x": "8", "y": "8", "width": "8", "height": "8", "rx": "1" }
      ]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function File_text($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "file-text",
    "size": 24,
    "node": [
      [
        "path",
        {
          "d": "M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z"
        }
      ],
      ["path", { "d": "M14 2v5a1 1 0 0 0 1 1h5" }],
      ["path", { "d": "M10 9H8" }],
      ["path", { "d": "M16 13H8" }],
      ["path", { "d": "M16 17H8" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Folder_git_2($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "folder-git-2",
    "size": 24,
    "node": [
      ["path", { "d": "M18 19a5 5 0 0 1-5-5v8" }],
      [
        "path",
        {
          "d": "M9 20H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H20a2 2 0 0 1 2 2v5"
        }
      ],
      ["circle", { "cx": "13", "cy": "12", "r": "2" }],
      ["circle", { "cx": "20", "cy": "19", "r": "2" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Gauge($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "gauge",
    "size": 24,
    "node": [
      ["path", { "d": "m12 14 4-4" }],
      ["path", { "d": "M3.34 19a10 10 0 1 1 17.32 0" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Git_branch($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "git-branch",
    "size": 24,
    "node": [
      ["path", { "d": "M15 6a9 9 0 0 0-9 9V3" }],
      ["circle", { "cx": "18", "cy": "6", "r": "3" }],
      ["circle", { "cx": "6", "cy": "18", "r": "3" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Inbox($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "inbox",
    "size": 24,
    "node": [
      [
        "polyline",
        { "points": "22 12 16 12 14 15 10 15 8 12 2 12" }
      ],
      [
        "path",
        {
          "d": "M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"
        }
      ]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Info($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "info",
    "size": 24,
    "node": [
      ["circle", { "cx": "12", "cy": "12", "r": "10" }],
      ["path", { "d": "M12 16v-4" }],
      ["path", { "d": "M12 8h.01" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Layers($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "layers",
    "size": 24,
    "node": [
      [
        "path",
        {
          "d": "M12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83z"
        }
      ],
      [
        "path",
        {
          "d": "M2 12a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 12"
        }
      ],
      [
        "path",
        {
          "d": "M2 17a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 17"
        }
      ]
    ],
    "aliases": ["layers-3"]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Layout_dashboard($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "layout-dashboard",
    "size": 24,
    "node": [
      [
        "rect",
        { "width": "7", "height": "9", "x": "3", "y": "3", "rx": "1" }
      ],
      [
        "rect",
        { "width": "7", "height": "5", "x": "14", "y": "3", "rx": "1" }
      ],
      [
        "rect",
        { "width": "7", "height": "9", "x": "14", "y": "12", "rx": "1" }
      ],
      [
        "rect",
        { "width": "7", "height": "5", "x": "3", "y": "16", "rx": "1" }
      ]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function List_end($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "list-end",
    "size": 24,
    "node": [
      ["path", { "d": "M16 5H3" }],
      ["path", { "d": "M16 12H3" }],
      ["path", { "d": "M9 19H3" }],
      ["path", { "d": "m16 16-3 3 3 3" }],
      ["path", { "d": "M21 5v12a2 2 0 0 1-2 2h-6" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Loader($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "loader",
    "size": 24,
    "node": [
      ["path", { "d": "M12 2v4" }],
      ["path", { "d": "m16.2 7.8 2.9-2.9" }],
      ["path", { "d": "M18 12h4" }],
      ["path", { "d": "m16.2 16.2 2.9 2.9" }],
      ["path", { "d": "M12 18v4" }],
      ["path", { "d": "m4.9 19.1 2.9-2.9" }],
      ["path", { "d": "M2 12h4" }],
      ["path", { "d": "m4.9 4.9 2.9 2.9" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Menu($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "menu",
    "size": 24,
    "node": [
      ["path", { "d": "M4 5h16" }],
      ["path", { "d": "M4 12h16" }],
      ["path", { "d": "M4 19h16" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Message_square($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "message-square",
    "size": 24,
    "node": [
      [
        "path",
        {
          "d": "M22 17a2 2 0 0 1-2 2H6.828a2 2 0 0 0-1.414.586l-2.202 2.202A.71.71 0 0 1 2 21.286V5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2z"
        }
      ]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Pencil($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "pencil",
    "size": 24,
    "node": [
      [
        "path",
        {
          "d": "M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z"
        }
      ],
      ["path", { "d": "m15 5 4 4" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Plug($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "plug",
    "size": 24,
    "node": [
      ["path", { "d": "M12 22v-5" }],
      ["path", { "d": "M15 8V2" }],
      [
        "path",
        {
          "d": "M17 8a1 1 0 0 1 1 1v4a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V9a1 1 0 0 1 1-1z"
        }
      ],
      ["path", { "d": "M9 8V2" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Refresh_cw($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "refresh-cw",
    "size": 24,
    "node": [
      [
        "path",
        { "d": "M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8" }
      ],
      ["path", { "d": "M21 3v5h-5" }],
      [
        "path",
        { "d": "M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16" }
      ],
      ["path", { "d": "M8 16H3v5" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Rotate_ccw($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "rotate-ccw",
    "size": 24,
    "node": [
      [
        "path",
        { "d": "M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" }
      ],
      ["path", { "d": "M3 3v5h5" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Share_2($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "share-2",
    "size": 24,
    "node": [
      ["circle", { "cx": "18", "cy": "5", "r": "3" }],
      ["circle", { "cx": "6", "cy": "12", "r": "3" }],
      ["circle", { "cx": "18", "cy": "19", "r": "3" }],
      [
        "line",
        { "x1": "8.59", "x2": "15.42", "y1": "13.51", "y2": "17.49" }
      ],
      [
        "line",
        { "x1": "15.41", "x2": "8.59", "y1": "6.51", "y2": "10.49" }
      ]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Shrink($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "shrink",
    "size": 24,
    "node": [
      ["path", { "d": "m15 15 6 6m-6-6v4.8m0-4.8h4.8" }],
      ["path", { "d": "M9 19.8V15m0 0H4.2M9 15l-6 6" }],
      ["path", { "d": "M15 4.2V9m0 0h4.8M15 9l6-6" }],
      ["path", { "d": "M9 4.2V9m0 0H4.2M9 9 3 3" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Signpost($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "signpost",
    "size": 24,
    "node": [
      ["path", { "d": "M12 13v8" }],
      ["path", { "d": "M12 3v3" }],
      [
        "path",
        {
          "d": "M2.354 10.354a1.207 1.207 0 0 1 0-1.708l2.06-2.06A2 2 0 0 1 5.828 6h12.344a2 2 0 0 1 1.414.586l2.06 2.06a1.207 1.207 0 0 1 0 1.708l-2.06 2.06a2 2 0 0 1-1.414.586H5.828a2 2 0 0 1-1.414-.586z"
        }
      ]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function Ticket($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "ticket",
    "size": 24,
    "node": [
      [
        "path",
        {
          "d": "M2 9a3 3 0 0 1 0 6v2a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-2a3 3 0 0 1 0-6V7a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2Z"
        }
      ],
      ["path", { "d": "M13 5v2" }],
      ["path", { "d": "M13 17v2" }],
      ["path", { "d": "M13 11v2" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
}
function X($$renderer, $$props) {
  let { $$slots, $$events, ...props } = $$props;
  const iconData = {
    "name": "x",
    "size": 24,
    "node": [
      ["path", { "d": "M18 6 6 18" }],
      ["path", { "d": "m6 6 12 12" }]
    ]
  };
  Icon($$renderer, spread_props([props, { icon: iconData }]));
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
    for (let index2 = 0; index2 < len; index2++) {
      const currentCharacter = className[index2];
      if (bracketDepth === 0 && parenDepth === 0) {
        if (currentCharacter === MODIFIER_SEPARATOR) {
          modifiers.push(className.slice(modifierStart, index2));
          modifierStart = index2 + 1;
          continue;
        }
        if (currentCharacter === "/") {
          postfixModifierPosition = index2;
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
  config.orderSensitiveModifiers.forEach((mod, index2) => {
    modifierWeights.set(mod, 1e6 + index2);
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
  for (let index2 = classNames.length - 1; index2 >= 0; index2 -= 1) {
    const originalClassName = classNames[index2];
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
  let index2 = 0;
  let argument;
  let resolvedValue;
  let string = "";
  while (index2 < classLists.length) {
    if (argument = classLists[index2++]) {
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
const mergeConfigs$1 = (baseConfig, {
  cacheSize,
  prefix,
  experimentalParseClassName,
  extend = {},
  override = {}
}) => {
  overrideProperty$1(baseConfig, "cacheSize", cacheSize);
  overrideProperty$1(baseConfig, "prefix", prefix);
  overrideProperty$1(baseConfig, "experimentalParseClassName", experimentalParseClassName);
  overrideConfigProperties$1(baseConfig.theme, override.theme);
  overrideConfigProperties$1(baseConfig.classGroups, override.classGroups);
  overrideConfigProperties$1(baseConfig.conflictingClassGroups, override.conflictingClassGroups);
  overrideConfigProperties$1(baseConfig.conflictingClassGroupModifiers, override.conflictingClassGroupModifiers);
  overrideProperty$1(baseConfig, "postfixLookupClassGroups", override.postfixLookupClassGroups);
  overrideProperty$1(baseConfig, "orderSensitiveModifiers", override.orderSensitiveModifiers);
  mergeConfigProperties$1(baseConfig.theme, extend.theme);
  mergeConfigProperties$1(baseConfig.classGroups, extend.classGroups);
  mergeConfigProperties$1(baseConfig.conflictingClassGroups, extend.conflictingClassGroups);
  mergeConfigProperties$1(baseConfig.conflictingClassGroupModifiers, extend.conflictingClassGroupModifiers);
  mergeArrayProperties$1(baseConfig, extend, "postfixLookupClassGroups");
  mergeArrayProperties$1(baseConfig, extend, "orderSensitiveModifiers");
  return baseConfig;
};
const overrideProperty$1 = (baseObject, overrideKey, overrideValue) => {
  if (overrideValue !== void 0) {
    baseObject[overrideKey] = overrideValue;
  }
};
const overrideConfigProperties$1 = (baseObject, overrideObject) => {
  if (overrideObject) {
    for (const key in overrideObject) {
      overrideProperty$1(baseObject, key, overrideObject[key]);
    }
  }
};
const mergeConfigProperties$1 = (baseObject, mergeObject) => {
  if (mergeObject) {
    for (const key in mergeObject) {
      mergeArrayProperties$1(baseObject, mergeObject, key);
    }
  }
};
const mergeArrayProperties$1 = (baseObject, mergeObject, key) => {
  const mergeValue = mergeObject[key];
  if (mergeValue !== void 0) {
    baseObject[key] = baseObject[key] ? baseObject[key].concat(mergeValue) : mergeValue;
  }
};
const extendTailwindMerge = (configExtension, ...createConfig) => typeof configExtension === "function" ? createTailwindMerge$1(getDefaultConfig$1, configExtension, ...createConfig) : createTailwindMerge$1(() => mergeConfigs$1(getDefaultConfig$1(), configExtension), ...createConfig);
const twMergeConfig = {
  extend: {
    classGroups: {
      "font-size": [{ text: ["headline", "title", "body", "caption", "numeric"] }],
      shadow: [{ shadow: ["elev-1", "elev-2", "elev-3"] }]
    }
  }
};
const twMerge = extendTailwindMerge(twMergeConfig);
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
    for (let index2 = 0; index2 < length; index2++) {
      const item = value[index2];
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
  let size2 = 0;
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
            if (!promoteInner.has(overrideKey)) size2++;
            promoteInner.set(overrideKey, value);
            return value;
          }
        }
      }
      return CACHE_MISS;
    },
    set(coreKey, overrideKey, value) {
      if (size2 >= limit) {
        secondary = primary;
        primary = /* @__PURE__ */ new Map();
        size2 = 0;
      }
      let inner = primary.get(coreKey);
      if (!inner) {
        inner = /* @__PURE__ */ new Map();
        primary.set(coreKey, inner);
      }
      if (!inner.has(overrideKey)) size2++;
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
  const index2 = {};
  for (let i = 0; i < compiledCompoundSlots.length; i++) {
    const compoundSlot = compiledCompoundSlots[i];
    const slots = compoundSlot.source.slots;
    if (!Array.isArray(slots)) continue;
    for (let j = 0; j < slots.length; j++) {
      const slotKey = slots[j];
      if (!index2[slotKey]) index2[slotKey] = [];
      index2[slotKey].push(compoundSlot);
    }
  }
  return index2;
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
  for (let index2 = 0; index2 < validatorsLength; index2++) {
    const validatorObject = validators[index2];
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
  for (let index2 = 0; index2 < length; index2++) {
    const classDefinition = classGroup[index2];
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
  for (let index2 = 0; index2 < length; index2++) {
    const [key, value] = entries[index2];
    processClassesRecursively(value, getPart(classPartObject, key), classGroupId, theme);
  }
};
var getPart = (classPartObject, path) => {
  let current = classPartObject;
  const parts = path.split(CLASS_PART_SEPARATOR);
  const length = parts.length;
  for (let index2 = 0; index2 < length; index2++) {
    const part = parts[index2];
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
  for (let index2 = 0; index2 < len; index2++) {
    const charCode = className.charCodeAt(index2);
    if (bracketDepth === 0 && parenDepth === 0) {
      if (charCode === CHAR_MODIFIER_SEPARATOR) {
        modifiers.push(className.slice(modifierStart, index2));
        modifierStart = index2 + 1;
        continue;
      }
      if (charCode === CHAR_POSTFIX_SEPARATOR) {
        postfixModifierPosition = index2;
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
    for (let index2 = 0; index2 < modifiers.length; index2++) {
      const modifier = modifiers[index2];
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
    for (let index2 = 0; index2 < length; index2++) {
      const charCode = classList.charCodeAt(index2);
      if (charCode === 32) {
        if (tokenStart !== -1) {
          tokens.push(classList.slice(tokenStart, index2));
          tokenStart = -1;
        }
      } else if (charCode >= 9 && charCode <= 13) {
        splitSawNonSpaceWhitespace = true;
        if (tokenStart !== -1) {
          tokens.push(classList.slice(tokenStart, index2));
          tokenStart = -1;
        }
      } else if (tokenStart === -1) {
        tokenStart = index2;
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
    for (let index2 = 0; index2 < conflictGroups.length; index2++) {
      conflictIds.push(internConflictKey(modifierId + conflictGroups[index2]));
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
    for (let index2 = classCount - 1; index2 >= 0; index2 -= 1) {
      const className = classNames[index2];
      tokenCharCount += className.length;
      const descriptor = getClassDescriptor(className);
      if (descriptor.isExternal) {
        keepFlags[index2] = 1;
        continue;
      }
      const classId = descriptor.classId;
      if (claimedGeneration[classId] === generation) {
        keepFlags[index2] = 0;
        didDrop = true;
        continue;
      }
      claimedGeneration[classId] = generation;
      const conflictIds = descriptor.conflictIds;
      for (let conflictIndex = 0; conflictIndex < conflictIds.length; conflictIndex++) {
        claimedGeneration[conflictIds[conflictIndex]] = generation;
      }
      keepFlags[index2] = 1;
    }
    if (!didDrop && !splitSawNonSpaceWhitespace && classList.length === tokenCharCount + classCount - 1) {
      return classList;
    }
    let result = "";
    for (let index2 = 0; index2 < classCount; index2++) {
      if (keepFlags[index2] === 1) {
        if (result) result += " ";
        result += classNames[index2];
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
    for (let index2 = 0; index2 < classGroupIds.length; index2++) {
      lookup[classGroupIds[index2]] = true;
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
      size: size2 = "default",
      type = "button",
      children,
      $$slots,
      $$events,
      ...rest
    } = $$props;
    $$renderer2.push(`<button${attributes({
      type,
      class: clsx(cn(buttonVariants({ variant, size: size2 }), className)),
      ...rest
    })}>`);
    children == null ? void 0 : children($$renderer2);
    $$renderer2.push(`<!----></button>`);
  });
}
function isFunction$1(value) {
  return typeof value === "function";
}
function isObject(value) {
  return value !== null && typeof value === "object";
}
const CLASS_VALUE_PRIMITIVE_TYPES = ["string", "number", "bigint", "boolean"];
function isClassValue(value) {
  if (value === null || value === void 0)
    return true;
  if (CLASS_VALUE_PRIMITIVE_TYPES.includes(typeof value))
    return true;
  if (Array.isArray(value))
    return value.every((item) => isClassValue(item));
  if (typeof value === "object") {
    if (Object.getPrototypeOf(value) !== Object.prototype)
      return false;
    return true;
  }
  return false;
}
const BoxSymbol = Symbol("box");
const isWritableSymbol = Symbol("is-writable");
function boxWith(getter, setter) {
  const derived$1 = derived(getter);
  if (setter) {
    return {
      [BoxSymbol]: true,
      [isWritableSymbol]: true,
      get current() {
        return derived$1();
      },
      set current(v) {
        setter(v);
      }
    };
  }
  return {
    [BoxSymbol]: true,
    get current() {
      return getter();
    }
  };
}
function isBox(value) {
  return isObject(value) && BoxSymbol in value;
}
function boxFrom(value) {
  if (isBox(value)) return value;
  if (isFunction$1(value)) return boxWith(value);
  return simpleBox(value);
}
function simpleBox(initialValue) {
  let current = initialValue;
  return {
    [BoxSymbol]: true,
    [isWritableSymbol]: true,
    get current() {
      return current;
    },
    set current(v) {
      current = v;
    }
  };
}
function composeHandlers(...handlers) {
  return function(e) {
    var _a;
    for (const handler of handlers) {
      if (!handler)
        continue;
      if (e.defaultPrevented)
        return;
      if (typeof handler === "function") {
        handler.call(this, e);
      } else {
        (_a = handler.current) == null ? void 0 : _a.call(this, e);
      }
    }
  };
}
var COMMENT_REGEX = /\/\*[^*]*\*+([^/*][^*]*\*+)*\//g;
var NEWLINE_REGEX = /\n/g;
var WHITESPACE_REGEX = /^\s*/;
var PROPERTY_REGEX = /^(\*?[-#/*\\\w]+(\[[0-9a-z_-]+\])?)\s*/;
var COLON_REGEX = /^:\s*/;
var VALUE_REGEX = /^((?:'(?:\\'|.)*?'|"(?:\\"|.)*?"|\([^)]*?\)|[^};])+)/;
var SEMICOLON_REGEX = /^[;\s]*/;
var TRIM_REGEX = /^\s+|\s+$/g;
var NEWLINE = "\n";
var FORWARD_SLASH = "/";
var ASTERISK = "*";
var EMPTY_STRING = "";
var TYPE_COMMENT = "comment";
var TYPE_DECLARATION = "declaration";
function index(style, options) {
  if (typeof style !== "string") {
    throw new TypeError("First argument must be a string");
  }
  if (!style) return [];
  options = options || {};
  var lineno = 1;
  var column = 1;
  function updatePosition(str) {
    var lines = str.match(NEWLINE_REGEX);
    if (lines) lineno += lines.length;
    var i = str.lastIndexOf(NEWLINE);
    column = ~i ? str.length - i : column + str.length;
  }
  function position() {
    var start = { line: lineno, column };
    return function(node) {
      node.position = new Position(start);
      whitespace2();
      return node;
    };
  }
  function Position(start) {
    this.start = start;
    this.end = { line: lineno, column };
    this.source = options.source;
  }
  Position.prototype.content = style;
  function error(msg) {
    var err = new Error(
      options.source + ":" + lineno + ":" + column + ": " + msg
    );
    err.reason = msg;
    err.filename = options.source;
    err.line = lineno;
    err.column = column;
    err.source = style;
    if (options.silent) ;
    else {
      throw err;
    }
  }
  function match(re) {
    var m = re.exec(style);
    if (!m) return;
    var str = m[0];
    updatePosition(str);
    style = style.slice(str.length);
    return m;
  }
  function whitespace2() {
    match(WHITESPACE_REGEX);
  }
  function comments(rules) {
    var c;
    rules = rules || [];
    while (c = comment()) {
      if (c !== false) {
        rules.push(c);
      }
    }
    return rules;
  }
  function comment() {
    var pos = position();
    if (FORWARD_SLASH != style.charAt(0) || ASTERISK != style.charAt(1)) return;
    var i = 2;
    while (EMPTY_STRING != style.charAt(i) && (ASTERISK != style.charAt(i) || FORWARD_SLASH != style.charAt(i + 1))) {
      ++i;
    }
    i += 2;
    if (EMPTY_STRING === style.charAt(i - 1)) {
      return error("End of comment missing");
    }
    var str = style.slice(2, i - 2);
    column += 2;
    updatePosition(str);
    style = style.slice(i);
    column += 2;
    return pos({
      type: TYPE_COMMENT,
      comment: str
    });
  }
  function declaration() {
    var pos = position();
    var prop = match(PROPERTY_REGEX);
    if (!prop) return;
    comment();
    if (!match(COLON_REGEX)) return error("property missing ':'");
    var val = match(VALUE_REGEX);
    var ret = pos({
      type: TYPE_DECLARATION,
      property: trim(prop[0].replace(COMMENT_REGEX, EMPTY_STRING)),
      value: val ? trim(val[0].replace(COMMENT_REGEX, EMPTY_STRING)) : EMPTY_STRING
    });
    match(SEMICOLON_REGEX);
    return ret;
  }
  function declarations() {
    var decls = [];
    comments(decls);
    var decl;
    while (decl = declaration()) {
      if (decl !== false) {
        decls.push(decl);
        comments(decls);
      }
    }
    return decls;
  }
  whitespace2();
  return declarations();
}
function trim(str) {
  return str ? str.replace(TRIM_REGEX, EMPTY_STRING) : EMPTY_STRING;
}
function StyleToObject(style, iterator) {
  let styleObject = null;
  if (!style || typeof style !== "string") {
    return styleObject;
  }
  const declarations = index(style);
  const hasIterator = typeof iterator === "function";
  declarations.forEach((declaration) => {
    if (declaration.type !== "declaration") {
      return;
    }
    const { property, value } = declaration;
    if (hasIterator) {
      iterator(property, value, declaration);
    } else if (value) {
      styleObject = styleObject || {};
      styleObject[property] = value;
    }
  });
  return styleObject;
}
const NUMBER_CHAR_RE = /\d/;
const STR_SPLITTERS = ["-", "_", "/", "."];
function isUppercase(char = "") {
  if (NUMBER_CHAR_RE.test(char))
    return void 0;
  return char !== char.toLowerCase();
}
function splitByCase(str) {
  const parts = [];
  let buff = "";
  let previousUpper;
  let previousSplitter;
  for (const char of str) {
    const isSplitter = STR_SPLITTERS.includes(char);
    if (isSplitter === true) {
      parts.push(buff);
      buff = "";
      previousUpper = void 0;
      continue;
    }
    const isUpper = isUppercase(char);
    if (previousSplitter === false) {
      if (previousUpper === false && isUpper === true) {
        parts.push(buff);
        buff = char;
        previousUpper = isUpper;
        continue;
      }
      if (previousUpper === true && isUpper === false && buff.length > 1) {
        const lastChar = buff.at(-1);
        parts.push(buff.slice(0, Math.max(0, buff.length - 1)));
        buff = lastChar + char;
        previousUpper = isUpper;
        continue;
      }
    }
    buff += char;
    previousUpper = isUpper;
    previousSplitter = isSplitter;
  }
  parts.push(buff);
  return parts;
}
function pascalCase(str) {
  if (!str)
    return "";
  return splitByCase(str).map((p) => upperFirst(p)).join("");
}
function camelCase(str) {
  return lowerFirst(pascalCase(str || ""));
}
function upperFirst(str) {
  return str ? str[0].toUpperCase() + str.slice(1) : "";
}
function lowerFirst(str) {
  return str ? str[0].toLowerCase() + str.slice(1) : "";
}
function cssToStyleObj(css) {
  if (!css)
    return {};
  const styleObj = {};
  function iterator(name, value) {
    if (name.startsWith("-moz-") || name.startsWith("-webkit-") || name.startsWith("-ms-") || name.startsWith("-o-")) {
      styleObj[pascalCase(name)] = value;
      return;
    }
    if (name.startsWith("--")) {
      styleObj[name] = value;
      return;
    }
    styleObj[camelCase(name)] = value;
  }
  StyleToObject(css, iterator);
  return styleObj;
}
function executeCallbacks(...callbacks) {
  return (...args) => {
    for (const callback of callbacks) {
      if (typeof callback === "function") {
        callback(...args);
      }
    }
  };
}
function createParser(matcher, replacer) {
  const regex = RegExp(matcher, "g");
  return (str) => {
    if (typeof str !== "string") {
      throw new TypeError(`expected an argument of type string, but got ${typeof str}`);
    }
    if (!str.match(regex))
      return str;
    return str.replace(regex, replacer);
  };
}
const camelToKebab = createParser(/[A-Z]/, (match) => `-${match.toLowerCase()}`);
function styleToCSS(styleObj) {
  if (!styleObj || typeof styleObj !== "object" || Array.isArray(styleObj)) {
    throw new TypeError(`expected an argument of type object, but got ${typeof styleObj}`);
  }
  return Object.keys(styleObj).map((property) => `${camelToKebab(property)}: ${styleObj[property]};`).join("\n");
}
function styleToString(style = {}) {
  return styleToCSS(style).replace("\n", " ");
}
const EVENT_LIST = [
  "onabort",
  "onanimationcancel",
  "onanimationend",
  "onanimationiteration",
  "onanimationstart",
  "onauxclick",
  "onbeforeinput",
  "onbeforetoggle",
  "onblur",
  "oncancel",
  "oncanplay",
  "oncanplaythrough",
  "onchange",
  "onclick",
  "onclose",
  "oncompositionend",
  "oncompositionstart",
  "oncompositionupdate",
  "oncontextlost",
  "oncontextmenu",
  "oncontextrestored",
  "oncopy",
  "oncuechange",
  "oncut",
  "ondblclick",
  "ondrag",
  "ondragend",
  "ondragenter",
  "ondragleave",
  "ondragover",
  "ondragstart",
  "ondrop",
  "ondurationchange",
  "onemptied",
  "onended",
  "onerror",
  "onfocus",
  "onfocusin",
  "onfocusout",
  "onformdata",
  "ongotpointercapture",
  "oninput",
  "oninvalid",
  "onkeydown",
  "onkeypress",
  "onkeyup",
  "onload",
  "onloadeddata",
  "onloadedmetadata",
  "onloadstart",
  "onlostpointercapture",
  "onmousedown",
  "onmouseenter",
  "onmouseleave",
  "onmousemove",
  "onmouseout",
  "onmouseover",
  "onmouseup",
  "onpaste",
  "onpause",
  "onplay",
  "onplaying",
  "onpointercancel",
  "onpointerdown",
  "onpointerenter",
  "onpointerleave",
  "onpointermove",
  "onpointerout",
  "onpointerover",
  "onpointerup",
  "onprogress",
  "onratechange",
  "onreset",
  "onresize",
  "onscroll",
  "onscrollend",
  "onsecuritypolicyviolation",
  "onseeked",
  "onseeking",
  "onselect",
  "onselectionchange",
  "onselectstart",
  "onslotchange",
  "onstalled",
  "onsubmit",
  "onsuspend",
  "ontimeupdate",
  "ontoggle",
  "ontouchcancel",
  "ontouchend",
  "ontouchmove",
  "ontouchstart",
  "ontransitioncancel",
  "ontransitionend",
  "ontransitionrun",
  "ontransitionstart",
  "onvolumechange",
  "onwaiting",
  "onwebkitanimationend",
  "onwebkitanimationiteration",
  "onwebkitanimationstart",
  "onwebkittransitionend",
  "onwheel"
];
const EVENT_LIST_SET = new Set(EVENT_LIST);
function isEventHandler(key) {
  return EVENT_LIST_SET.has(key);
}
function mergeProps(...args) {
  const result = { ...args[0] };
  for (let i = 1; i < args.length; i++) {
    const props = args[i];
    if (!props)
      continue;
    for (const key of Object.keys(props)) {
      const a = result[key];
      const b = props[key];
      const aIsFunction = typeof a === "function";
      const bIsFunction = typeof b === "function";
      if (aIsFunction && typeof bIsFunction && isEventHandler(key)) {
        const aHandler = a;
        const bHandler = b;
        result[key] = composeHandlers(aHandler, bHandler);
      } else if (aIsFunction && bIsFunction) {
        result[key] = executeCallbacks(a, b);
      } else if (key === "class") {
        const aIsClassValue = isClassValue(a);
        const bIsClassValue = isClassValue(b);
        if (aIsClassValue && bIsClassValue) {
          result[key] = clsx$1(a, b);
        } else if (aIsClassValue) {
          result[key] = clsx$1(a);
        } else if (bIsClassValue) {
          result[key] = clsx$1(b);
        }
      } else if (key === "style") {
        const aIsObject = typeof a === "object";
        const bIsObject = typeof b === "object";
        const aIsString = typeof a === "string";
        const bIsString = typeof b === "string";
        if (aIsObject && bIsObject) {
          result[key] = { ...a, ...b };
        } else if (aIsObject && bIsString) {
          const parsedStyle = cssToStyleObj(b);
          result[key] = { ...a, ...parsedStyle };
        } else if (aIsString && bIsObject) {
          const parsedStyle = cssToStyleObj(a);
          result[key] = { ...parsedStyle, ...b };
        } else if (aIsString && bIsString) {
          const parsedStyleA = cssToStyleObj(a);
          const parsedStyleB = cssToStyleObj(b);
          result[key] = { ...parsedStyleA, ...parsedStyleB };
        } else if (aIsObject) {
          result[key] = a;
        } else if (bIsObject) {
          result[key] = b;
        } else if (aIsString) {
          result[key] = a;
        } else if (bIsString) {
          result[key] = b;
        }
      } else {
        result[key] = b !== void 0 ? b : a;
      }
    }
    for (const key of Object.getOwnPropertySymbols(props)) {
      const a = result[key];
      const b = props[key];
      result[key] = b !== void 0 ? b : a;
    }
  }
  if (typeof result.style === "object") {
    result.style = styleToString(result.style).replaceAll("\n", " ");
  }
  if (result.hidden === false) {
    result.hidden = void 0;
    delete result.hidden;
  }
  if (result.disabled === false) {
    result.disabled = void 0;
    delete result.disabled;
  }
  return result;
}
const defaultWindow = void 0;
function getActiveElement$1(document2) {
  let activeElement = document2.activeElement;
  while (activeElement == null ? void 0 : activeElement.shadowRoot) {
    const node = activeElement.shadowRoot.activeElement;
    if (node === activeElement)
      break;
    else
      activeElement = node;
  }
  return activeElement;
}
const event_symbol = Symbol("events");
function create_event(event_name, dom, handler, options = {}) {
  function target_handler(event) {
    if (!options.capture) {
      handle_event_propagation.call(dom, event);
    }
    if (!event.cancelBubble) {
      return without_reactive_context(() => {
        return handler == null ? void 0 : handler.call(this, event);
      });
    }
  }
  if (event_name.startsWith("pointer") || event_name.startsWith("touch") || event_name === "wheel") {
    target_handler.__removed = false;
    queue_micro_task(() => {
      if (!target_handler.__removed) {
        dom.addEventListener(event_name, target_handler, options);
      }
    });
  } else {
    dom.addEventListener(event_name, target_handler, options);
  }
  return target_handler;
}
function on(element2, type, handler, options = {}) {
  var target_handler = create_event(type, element2, handler, options);
  return () => {
    target_handler.__removed = true;
    element2.removeEventListener(type, target_handler, options);
  };
}
let last_propagated_event = null;
let last_propagated_event_clear_scheduled = false;
function handle_event_propagation(event) {
  var _a, _b;
  var handler_element = this;
  var owner_document = (
    /** @type {Node} */
    handler_element.ownerDocument
  );
  var event_name = event.type;
  var path = ((_a = event.composedPath) == null ? void 0 : _a.call(event)) || [];
  var current_target = (
    /** @type {null | Element} */
    path[0] || event.target
  );
  last_propagated_event = event;
  if (!last_propagated_event_clear_scheduled) {
    last_propagated_event_clear_scheduled = true;
    setTimeout(() => {
      last_propagated_event_clear_scheduled = false;
      last_propagated_event = null;
    });
  }
  var path_idx = 0;
  var handled_at = last_propagated_event === event && event[event_symbol];
  if (handled_at) {
    var at_idx = path.indexOf(handled_at);
    if (at_idx !== -1 && (handler_element === document || handler_element === /** @type {any} */
    window)) {
      event[event_symbol] = handler_element;
      return;
    }
    var handler_idx = path.indexOf(handler_element);
    if (handler_idx === -1) {
      return;
    }
    if (at_idx <= handler_idx) {
      path_idx = at_idx;
    }
  }
  current_target = /** @type {Element} */
  path[path_idx] || event.target;
  if (current_target === handler_element) return;
  define_property(event, "currentTarget", {
    configurable: true,
    get() {
      return current_target || owner_document;
    }
  });
  try {
    var throw_error;
    var other_errors = [];
    while (current_target !== null) {
      if (current_target === handler_element) break;
      try {
        var delegated = (_b = current_target[event_symbol]) == null ? void 0 : _b[event_name];
        if (delegated != null && (!/** @type {any} */
        current_target.disabled || // DOM could've been updated already by the time this is reached, so we check this as well
        // -> the target could not have been disabled because it emits the event in the first place
        event.target === current_target)) {
          delegated.call(current_target, event);
        }
      } catch (error) {
        if (throw_error) {
          other_errors.push(error);
        } else {
          throw_error = error;
        }
      }
      if (event.cancelBubble) break;
      path_idx++;
      current_target = path_idx < path.length ? (
        /** @type {Element} */
        path[path_idx]
      ) : null;
    }
    if (throw_error) {
      for (let error of other_errors) {
        queueMicrotask(() => {
          throw error;
        });
      }
      throw throw_error;
    }
  } finally {
    event[event_symbol] = handler_element;
    delete event.currentTarget;
  }
}
const SvelteMap = globalThis.Map;
function createSubscriber(_) {
  return () => {
  };
}
class ActiveElement {
  constructor(options = {}) {
    __privateAdd(this, _document);
    __privateAdd(this, _subscribe);
    const { window: window2 = defaultWindow, document: document2 = window2 == null ? void 0 : window2.document } = options;
    if (window2 === void 0) return;
    __privateSet(this, _document, document2);
    __privateSet(this, _subscribe, createSubscriber());
  }
  get current() {
    var _a;
    (_a = __privateGet(this, _subscribe)) == null ? void 0 : _a.call(this);
    if (!__privateGet(this, _document)) return null;
    return getActiveElement$1(__privateGet(this, _document));
  }
}
_document = new WeakMap();
_subscribe = new WeakMap();
new ActiveElement();
function isFunction(value) {
  return typeof value === "function";
}
class Context {
  /**
   * @param name The name of the context.
   * This is used for generating the context key and error messages.
   */
  constructor(name) {
    __privateAdd(this, _name);
    __privateAdd(this, _key);
    __privateSet(this, _name, name);
    __privateSet(this, _key, Symbol(name));
  }
  /**
   * The key used to get and set the context.
   *
   * It is not recommended to use this value directly.
   * Instead, use the methods provided by this class.
   */
  get key() {
    return __privateGet(this, _key);
  }
  /**
   * Checks whether this has been set in the context of a parent component.
   *
   * Must be called during component initialisation.
   */
  exists() {
    return hasContext(__privateGet(this, _key));
  }
  /**
   * Retrieves the context that belongs to the closest parent component.
   *
   * Must be called during component initialisation.
   *
   * @throws An error if the context does not exist.
   */
  get() {
    const context2 = getContext(__privateGet(this, _key));
    if (context2 === void 0) {
      throw new Error(`Context "${__privateGet(this, _name)}" not found`);
    }
    return context2;
  }
  /**
   * Retrieves the context that belongs to the closest parent component,
   * or the given fallback value if the context does not exist.
   *
   * Must be called during component initialisation.
   */
  getOr(fallback2) {
    const context2 = getContext(__privateGet(this, _key));
    if (context2 === void 0) {
      return fallback2;
    }
    return context2;
  }
  /**
   * Associates the given value with the current component and returns it.
   *
   * Must be called during component initialisation.
   */
  set(context2) {
    return setContext(__privateGet(this, _key), context2);
  }
}
_name = new WeakMap();
_key = new WeakMap();
function runWatcher(sources, flush, effect, options = {}) {
  const { lazy = false } = options;
}
function watch(sources, effect, options) {
  runWatcher(sources, "post", effect, options);
}
function watchPre(sources, effect, options) {
  runWatcher(sources, "pre", effect, options);
}
watch.pre = watchPre;
function get$1(value) {
  if (isFunction(value)) {
    return value();
  }
  return value;
}
class ElementSize {
  constructor(node, options = { box: "border-box" }) {
    // no need to use `$state` here since we are using createSubscriber
    __privateAdd(this, _size, { width: 0, height: 0 });
    __privateAdd(this, _observed, false);
    __privateAdd(this, _options);
    __privateAdd(this, _node);
    __privateAdd(this, _window);
    // we use a derived here to extract the width so that if the width doesn't change we don't get a state update
    // which we would get if we would just use a getter since the version of the subscriber will be changing
    __privateAdd(this, _width, derived(() => {
      var _a;
      (_a = __privateGet(this, _subscribe2).call(this)) == null ? void 0 : _a();
      return this.getSize().width;
    }));
    // we use a derived here to extract the height so that if the height doesn't change we don't get a state update
    // which we would get if we would just use a getter since the version of the subscriber will be changing
    __privateAdd(this, _height, derived(() => {
      var _a;
      (_a = __privateGet(this, _subscribe2).call(this)) == null ? void 0 : _a();
      return this.getSize().height;
    }));
    // we need to use a derived here because the class will be created before the node is bound to the ref
    __privateAdd(this, _subscribe2, derived(() => {
      const node$ = get$1(__privateGet(this, _node));
      if (!node$) return;
      return createSubscriber();
    }));
    __privateSet(this, _window, options.window ?? defaultWindow);
    __privateSet(this, _options, options);
    __privateSet(this, _node, node);
    __privateSet(this, _size, { width: 0, height: 0 });
  }
  calculateSize() {
    const element2 = get$1(__privateGet(this, _node));
    if (!element2 || !__privateGet(this, _window)) {
      return;
    }
    const offsetWidth = element2.offsetWidth;
    const offsetHeight = element2.offsetHeight;
    if (__privateGet(this, _options).box === "border-box") {
      return { width: offsetWidth, height: offsetHeight };
    }
    const style = __privateGet(this, _window).getComputedStyle(element2);
    const paddingWidth = parseFloat(style.paddingLeft) + parseFloat(style.paddingRight);
    const paddingHeight = parseFloat(style.paddingTop) + parseFloat(style.paddingBottom);
    const borderWidth = parseFloat(style.borderLeftWidth) + parseFloat(style.borderRightWidth);
    const borderHeight = parseFloat(style.borderTopWidth) + parseFloat(style.borderBottomWidth);
    const contentWidth = offsetWidth - paddingWidth - borderWidth;
    const contentHeight = offsetHeight - paddingHeight - borderHeight;
    return { width: contentWidth, height: contentHeight };
  }
  getSize() {
    return __privateGet(this, _observed) ? __privateGet(this, _size) : this.calculateSize() ?? __privateGet(this, _size);
  }
  get current() {
    var _a;
    (_a = __privateGet(this, _subscribe2).call(this)) == null ? void 0 : _a();
    return this.getSize();
  }
  get width() {
    return __privateGet(this, _width).call(this);
  }
  get height() {
    return __privateGet(this, _height).call(this);
  }
}
_size = new WeakMap();
_observed = new WeakMap();
_options = new WeakMap();
_node = new WeakMap();
_window = new WeakMap();
_width = new WeakMap();
_height = new WeakMap();
_subscribe2 = new WeakMap();
function onDestroyEffect(fn) {
}
function afterTick(fn) {
  tick().then(fn);
}
const ELEMENT_NODE = 1;
const DOCUMENT_NODE = 9;
const DOCUMENT_FRAGMENT_NODE = 11;
function isHTMLElement$2(node) {
  return isObject(node) && node.nodeType === ELEMENT_NODE && typeof node.nodeName === "string";
}
function isDocument(node) {
  return isObject(node) && node.nodeType === DOCUMENT_NODE;
}
function isWindow(node) {
  var _a;
  return isObject(node) && ((_a = node.constructor) == null ? void 0 : _a.name) === "VisualViewport";
}
function isNode$1(node) {
  return isObject(node) && node.nodeType !== void 0;
}
function isShadowRoot$1(node) {
  return isNode$1(node) && node.nodeType === DOCUMENT_FRAGMENT_NODE && "host" in node;
}
function contains(parent, child) {
  var _a;
  if (!parent || !child)
    return false;
  if (!isHTMLElement$2(parent) || !isHTMLElement$2(child))
    return false;
  const rootNode = (_a = child.getRootNode) == null ? void 0 : _a.call(child);
  if (parent === child)
    return true;
  if (parent.contains(child))
    return true;
  if (rootNode && isShadowRoot$1(rootNode)) {
    let next = child;
    while (next) {
      if (parent === next)
        return true;
      next = next.parentNode || next.host;
    }
  }
  return false;
}
function getDocument(node) {
  if (isDocument(node))
    return node;
  if (isWindow(node))
    return node.document;
  return (node == null ? void 0 : node.ownerDocument) ?? document;
}
function getWindow$1(node) {
  var _a;
  if (isShadowRoot$1(node))
    return getWindow$1(node.host);
  if (isDocument(node))
    return node.defaultView ?? window;
  if (isHTMLElement$2(node))
    return ((_a = node.ownerDocument) == null ? void 0 : _a.defaultView) ?? window;
  return window;
}
function getActiveElement(rootNode) {
  let activeElement = rootNode.activeElement;
  while (activeElement == null ? void 0 : activeElement.shadowRoot) {
    const el = activeElement.shadowRoot.activeElement;
    if (el === activeElement)
      break;
    else
      activeElement = el;
  }
  return activeElement;
}
class DOMContext {
  constructor(element2) {
    __publicField(this, "element");
    __privateAdd(this, _root, derived(() => {
      if (!this.element.current) return document;
      const rootNode = this.element.current.getRootNode() ?? document;
      return rootNode;
    }));
    __publicField(this, "getDocument", () => {
      return getDocument(this.root);
    });
    __publicField(this, "getWindow", () => {
      return this.getDocument().defaultView ?? window;
    });
    __publicField(this, "getActiveElement", () => {
      return getActiveElement(this.root);
    });
    __publicField(this, "isActiveElement", (node) => {
      return node === this.getActiveElement();
    });
    __publicField(this, "querySelector", (selector) => {
      if (!this.root) return null;
      return this.root.querySelector(selector);
    });
    __publicField(this, "querySelectorAll", (selector) => {
      if (!this.root) return [];
      return this.root.querySelectorAll(selector);
    });
    __publicField(this, "setTimeout", (callback, delay) => {
      return this.getWindow().setTimeout(callback, delay);
    });
    __publicField(this, "clearTimeout", (timeoutId) => {
      return this.getWindow().clearTimeout(timeoutId);
    });
    if (typeof element2 === "function") {
      this.element = boxWith(element2);
    } else {
      this.element = element2;
    }
  }
  get root() {
    return __privateGet(this, _root).call(this);
  }
  set root($$value) {
    return __privateGet(this, _root).call(this, $$value);
  }
  getElementById(id) {
    return this.root.getElementById(id);
  }
}
_root = new WeakMap();
function createAttachmentKey() {
  return Symbol(ATTACHMENT_KEY);
}
function attachRef(ref, onChange) {
  return {
    [createAttachmentKey()]: (node) => {
      if (isBox(ref)) {
        ref.current = node;
        run(() => onChange == null ? void 0 : onChange(node));
        return () => {
          if ("isConnected" in node && node.isConnected)
            return;
          ref.current = null;
          onChange == null ? void 0 : onChange(null);
        };
      }
      ref(node);
      run(() => onChange == null ? void 0 : onChange(node));
      return () => {
        if ("isConnected" in node && node.isConnected)
          return;
        ref(null);
        onChange == null ? void 0 : onChange(null);
      };
    }
  };
}
function boolToStr(condition) {
  return condition ? "true" : "false";
}
function boolToEmptyStrOrUndef(condition) {
  return condition ? "" : void 0;
}
function getDataOpenClosed(condition) {
  return condition ? "open" : "closed";
}
function getDataTransitionAttrs(state2) {
  if (state2 === "starting")
    return { "data-starting-style": "" };
  if (state2 === "ending")
    return { "data-ending-style": "" };
  return {};
}
class BitsAttrs {
  constructor(config) {
    __privateAdd(this, _variant);
    __privateAdd(this, _prefix);
    __publicField(this, "attrs");
    __privateSet(this, _variant, config.getVariant ? config.getVariant() : null);
    __privateSet(this, _prefix, __privateGet(this, _variant) ? `data-${__privateGet(this, _variant)}-` : `data-${config.component}-`);
    this.getAttr = this.getAttr.bind(this);
    this.selector = this.selector.bind(this);
    this.attrs = Object.fromEntries(config.parts.map((part) => [part, this.getAttr(part)]));
  }
  getAttr(part, variantOverride) {
    if (variantOverride)
      return `data-${variantOverride}-${part}`;
    return `${__privateGet(this, _prefix)}${part}`;
  }
  selector(part, variantOverride) {
    return `[${this.getAttr(part, variantOverride)}]`;
  }
}
_variant = new WeakMap();
_prefix = new WeakMap();
function createBitsAttrs(config) {
  const bitsAttrs = new BitsAttrs(config);
  return {
    ...bitsAttrs.attrs,
    selector: bitsAttrs.selector,
    getAttr: bitsAttrs.getAttr
  };
}
const ENTER = "Enter";
const ESCAPE = "Escape";
const SPACE = " ";
const isBrowser = typeof document !== "undefined";
const isIOS = getIsIOS();
function getIsIOS() {
  var _a, _b;
  return isBrowser && ((_a = window == null ? void 0 : window.navigator) == null ? void 0 : _a.userAgent) && (/iP(ad|hone|od)/.test(window.navigator.userAgent) || // The new iPad Pro Gen3 does not identify itself as iPad, but as Macintosh.
  ((_b = window == null ? void 0 : window.navigator) == null ? void 0 : _b.maxTouchPoints) > 2 && /iPad|Macintosh/.test(window == null ? void 0 : window.navigator.userAgent));
}
function isHTMLElement$1(element2) {
  return element2 instanceof HTMLElement;
}
function isElement$1(element2) {
  return element2 instanceof Element;
}
function isElementOrSVGElement(element2) {
  return element2 instanceof Element || element2 instanceof SVGElement;
}
function isTouch(e) {
  return e.pointerType === "touch";
}
function isFocusVisible(element2) {
  return element2.matches(":focus-visible");
}
function isNotNull(value) {
  return value !== null;
}
class AnimationsComplete {
  constructor(opts) {
    __privateAdd(this, _AnimationsComplete_instances);
    __privateAdd(this, _opts);
    __privateAdd(this, _currentFrame, null);
    __privateAdd(this, _observer, null);
    __privateAdd(this, _runId, 0);
    __privateSet(this, _opts, opts);
  }
  run(fn) {
    __privateMethod(this, _AnimationsComplete_instances, cleanup_fn).call(this);
    const node = __privateGet(this, _opts).ref.current;
    if (!node)
      return;
    if (typeof node.getAnimations !== "function") {
      __privateMethod(this, _AnimationsComplete_instances, executeCallback_fn).call(this, fn);
      return;
    }
    const runId = __privateGet(this, _runId);
    const executeIfCurrent = () => {
      if (runId !== __privateGet(this, _runId))
        return;
      __privateMethod(this, _AnimationsComplete_instances, executeCallback_fn).call(this, fn);
    };
    const waitForAnimations = () => {
      if (runId !== __privateGet(this, _runId))
        return;
      const animations = node.getAnimations();
      if (animations.length === 0) {
        executeIfCurrent();
        return;
      }
      Promise.all(animations.map((animation) => animation.finished)).then(() => {
        executeIfCurrent();
      }).catch(() => {
        if (runId !== __privateGet(this, _runId))
          return;
        const currentAnimations = node.getAnimations();
        const hasRunningAnimations = currentAnimations.some((animation) => animation.pending || animation.playState !== "finished");
        if (hasRunningAnimations) {
          waitForAnimations();
          return;
        }
        executeIfCurrent();
      });
    };
    const requestWaitForAnimations = () => {
      __privateSet(this, _currentFrame, window.requestAnimationFrame(() => {
        __privateSet(this, _currentFrame, null);
        waitForAnimations();
      }));
    };
    if (!__privateGet(this, _opts).afterTick.current) {
      requestWaitForAnimations();
      return;
    }
    __privateSet(this, _currentFrame, window.requestAnimationFrame(() => {
      __privateSet(this, _currentFrame, null);
      const startingStyleAttr = "data-starting-style";
      if (!node.hasAttribute(startingStyleAttr)) {
        requestWaitForAnimations();
        return;
      }
      __privateSet(this, _observer, new MutationObserver(() => {
        var _a;
        if (runId !== __privateGet(this, _runId))
          return;
        if (node.hasAttribute(startingStyleAttr))
          return;
        (_a = __privateGet(this, _observer)) == null ? void 0 : _a.disconnect();
        __privateSet(this, _observer, null);
        requestWaitForAnimations();
      }));
      __privateGet(this, _observer).observe(node, {
        attributes: true,
        attributeFilter: [startingStyleAttr]
      });
    }));
  }
}
_opts = new WeakMap();
_currentFrame = new WeakMap();
_observer = new WeakMap();
_runId = new WeakMap();
_AnimationsComplete_instances = new WeakSet();
cleanup_fn = function() {
  var _a;
  if (__privateGet(this, _currentFrame) !== null) {
    window.cancelAnimationFrame(__privateGet(this, _currentFrame));
    __privateSet(this, _currentFrame, null);
  }
  (_a = __privateGet(this, _observer)) == null ? void 0 : _a.disconnect();
  __privateSet(this, _observer, null);
  __privateWrapper(this, _runId)._++;
};
executeCallback_fn = function(fn) {
  const execute = () => {
    fn();
  };
  if (__privateGet(this, _opts).afterTick) {
    afterTick(execute);
  } else {
    execute();
  }
};
class PresenceManager {
  constructor(opts) {
    __privateAdd(this, _PresenceManager_instances);
    __privateAdd(this, _opts2);
    __privateAdd(this, _enabled);
    __privateAdd(this, _afterAnimations);
    __privateAdd(this, _shouldRender, false);
    __privateAdd(this, _transitionStatus);
    __privateAdd(this, _hasMounted, false);
    __privateAdd(this, _transitionFrame, null);
    __privateSet(this, _opts2, opts);
    __privateSet(this, _shouldRender, opts.open.current);
    __privateSet(this, _enabled, opts.enabled ?? true);
    __privateSet(this, _afterAnimations, new AnimationsComplete({ ref: __privateGet(this, _opts2).ref, afterTick: __privateGet(this, _opts2).open }));
    watch(() => __privateGet(this, _opts2).open.current, (isOpen) => {
      var _a, _b, _c, _d, _e, _f;
      if (!__privateGet(this, _hasMounted)) {
        __privateSet(this, _hasMounted, true);
        return;
      }
      __privateMethod(this, _PresenceManager_instances, clearTransitionFrame_fn).call(this);
      if (!isOpen && ((_b = (_a = __privateGet(this, _opts2)).shouldSkipExitAnimation) == null ? void 0 : _b.call(_a))) {
        __privateSet(this, _shouldRender, false);
        __privateSet(this, _transitionStatus, void 0);
        (_d = (_c = __privateGet(this, _opts2)).onComplete) == null ? void 0 : _d.call(_c);
        return;
      }
      if (isOpen) __privateSet(this, _shouldRender, true);
      __privateSet(this, _transitionStatus, isOpen ? "starting" : "ending");
      if (isOpen) {
        __privateSet(this, _transitionFrame, window.requestAnimationFrame(() => {
          __privateSet(this, _transitionFrame, null);
          if (__privateGet(this, _opts2).open.current) {
            __privateSet(this, _transitionStatus, void 0);
          }
        }));
      }
      if (!__privateGet(this, _enabled)) {
        if (!isOpen) {
          __privateSet(this, _shouldRender, false);
        }
        __privateSet(this, _transitionStatus, void 0);
        (_f = (_e = __privateGet(this, _opts2)).onComplete) == null ? void 0 : _f.call(_e);
        return;
      }
      __privateGet(this, _afterAnimations).run(() => {
        var _a2, _b2;
        if (isOpen === __privateGet(this, _opts2).open.current) {
          if (!__privateGet(this, _opts2).open.current) {
            __privateSet(this, _shouldRender, false);
          }
          __privateSet(this, _transitionStatus, void 0);
          (_b2 = (_a2 = __privateGet(this, _opts2)).onComplete) == null ? void 0 : _b2.call(_a2);
        }
      });
    });
  }
  get shouldRender() {
    return __privateGet(this, _shouldRender);
  }
  get transitionStatus() {
    return __privateGet(this, _transitionStatus);
  }
}
_opts2 = new WeakMap();
_enabled = new WeakMap();
_afterAnimations = new WeakMap();
_shouldRender = new WeakMap();
_transitionStatus = new WeakMap();
_hasMounted = new WeakMap();
_transitionFrame = new WeakMap();
_PresenceManager_instances = new WeakSet();
clearTransitionFrame_fn = function() {
  if (__privateGet(this, _transitionFrame) === null) return;
  window.cancelAnimationFrame(__privateGet(this, _transitionFrame));
  __privateSet(this, _transitionFrame, null);
};
function noop() {
}
function createId(prefixOrUid, uid) {
  return `bits-${prefixOrUid}`;
}
const BitsConfigContext = new Context("BitsConfig");
function getBitsConfig() {
  const fallback2 = new BitsConfigState(null, {});
  return BitsConfigContext.getOr(fallback2).opts;
}
class BitsConfigState {
  constructor(parent, opts) {
    __publicField(this, "opts");
    const resolveConfigOption = createConfigResolver(parent, opts);
    this.opts = {
      defaultPortalTo: resolveConfigOption((config) => config.defaultPortalTo),
      defaultLocale: resolveConfigOption((config) => config.defaultLocale)
    };
  }
}
function createConfigResolver(parent, currentOpts) {
  return (getter) => {
    const configOption = boxWith(() => {
      var _a, _b;
      const value = (_a = getter(currentOpts)) == null ? void 0 : _a.current;
      if (value !== void 0)
        return value;
      if (parent === null)
        return void 0;
      return (_b = getter(parent.opts)) == null ? void 0 : _b.current;
    });
    return configOption;
  };
}
function createPropResolver(configOption, fallback2) {
  return (getProp) => {
    const config = getBitsConfig();
    return boxWith(() => {
      const propValue = getProp();
      if (propValue !== void 0)
        return propValue;
      const option = configOption(config).current;
      if (option !== void 0)
        return option;
      return fallback2;
    });
  };
}
const resolvePortalToProp = createPropResolver((config) => config.defaultPortalTo, "body");
function Portal($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let { to: toProp, children, disabled } = $$props;
    const to = resolvePortalToProp(() => toProp);
    getAllContexts();
    let target = derived(getTarget);
    function getTarget() {
      if (!isBrowser || disabled) return null;
      let localTarget = null;
      if (typeof to.current === "string") {
        const target2 = document.querySelector(to.current);
        localTarget = target2;
      } else {
        localTarget = to.current;
      }
      return localTarget;
    }
    let instance;
    function unmountInstance() {
      if (instance) {
        unmount();
        instance = null;
      }
    }
    watch([() => target(), () => disabled], ([target2, disabled2]) => {
      if (!target2 || disabled2) {
        unmountInstance();
        return;
      }
      instance = mount();
      return () => {
        unmountInstance();
      };
    });
    if (disabled) {
      $$renderer2.push("<!--[0-->");
      children == null ? void 0 : children($$renderer2);
      $$renderer2.push(`<!---->`);
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]-->`);
  });
}
function debounce(fn, wait = 500) {
  let timeout = null;
  const debounced = (...args) => {
    if (timeout !== null) {
      clearTimeout(timeout);
    }
    timeout = setTimeout(() => {
      fn(...args);
    }, wait);
  };
  debounced.destroy = () => {
    if (timeout !== null) {
      clearTimeout(timeout);
      timeout = null;
    }
  };
  return debounced;
}
function isOrContainsTarget(node, target) {
  return node === target || node.contains(target);
}
function getOwnerDocument(el) {
  return (el == null ? void 0 : el.ownerDocument) ?? document;
}
function isClickTrulyOutside(event, contentNode) {
  const { clientX, clientY } = event;
  const rect = contentNode.getBoundingClientRect();
  return clientX < rect.left || clientX > rect.right || clientY < rect.top || clientY > rect.bottom;
}
const CONTEXT_MENU_TRIGGER_ATTR = "data-context-menu-trigger";
const CONTEXT_MENU_CONTENT_ATTR = "data-context-menu-content";
globalThis.bitsDismissableLayers ?? (globalThis.bitsDismissableLayers = /* @__PURE__ */ new Map());
const _DismissibleLayerState = class _DismissibleLayerState {
  constructor(opts) {
    __privateAdd(this, _DismissibleLayerState_instances);
    __publicField(this, "opts");
    __privateAdd(this, _interactOutsideProp);
    __privateAdd(this, _behaviorType);
    __privateAdd(this, _interceptedEvents, { pointerdown: false });
    __privateAdd(this, _isResponsibleLayer, false);
    __privateAdd(this, _capturedPointerDown, null);
    __privateAdd(this, _isFocusInsideDOMTree, false);
    __privateAdd(this, _documentObj);
    __privateAdd(this, _onFocusOutside);
    __privateAdd(this, _unsubClickListener, noop);
    /**
     * Set once the layer is torn down. Deferred work scheduled before teardown can
     * still run afterwards, so every such callback must short-circuit on this
     * before reading `this.opts.ref.current` — reading a destroyed `$derived`
     * triggers Svelte's `derived_inert` warning. A class field rather than a
     * constructor local so the class-field handlers below can see it too.
     */
    __privateAdd(this, _destroyed, false);
    __privateAdd(this, _handleFocus, (event) => {
      if (event.defaultPrevented) return;
      if (__privateGet(this, _destroyed) || !this.opts.ref.current) return;
      afterTick(() => {
        var _a, _b;
        if (__privateGet(this, _destroyed)) return;
        if (!this.opts.ref.current || __privateGet(this, _isTargetWithinLayer).call(this, event.target)) return;
        if (event.target && !__privateGet(this, _isFocusInsideDOMTree)) {
          (_b = (_a = __privateGet(this, _onFocusOutside)).current) == null ? void 0 : _b.call(_a, event);
        }
      });
    });
    __privateAdd(this, _handleDismiss, (e) => {
      let event = e;
      if (event.defaultPrevented) {
        event = createWrappedEvent(e);
      }
      __privateGet(this, _interactOutsideProp).current(e);
    });
    __privateAdd(this, _interactOutside, (e) => {
      if (!this.opts.ref.current) {
        __privateGet(this, _unsubClickListener).call(this);
        return;
      }
      const isEventValid = this.opts.isValidEvent.current(e, this.opts.ref.current) || isValidEvent(e, this.opts.ref.current);
      if (!__privateGet(this, _isResponsibleLayer) || __privateMethod(this, _DismissibleLayerState_instances, isAnyEventIntercepted_fn).call(this) || !isEventValid) {
        __privateGet(this, _unsubClickListener).call(this);
        return;
      }
      let event = e;
      if (event.defaultPrevented) {
        event = createWrappedEvent(event);
      }
      if (__privateGet(this, _behaviorType).current !== "close" && __privateGet(this, _behaviorType).current !== "defer-otherwise-close") {
        __privateGet(this, _unsubClickListener).call(this);
        return;
      }
      if (e.pointerType === "touch") {
        __privateGet(this, _unsubClickListener).call(this);
        let backup = null;
        let unsubClick = noop;
        const dismiss = (event2) => {
          unsubClick();
          __privateGet(this, _handleDismiss).call(this, event2);
        };
        unsubClick = executeCallbacks(
          on(__privateGet(this, _documentObj), "click", dismiss, { once: true }),
          on(
            __privateGet(this, _documentObj),
            "click",
            (event2) => {
              backup = setTimeout(() => dismiss(event2), 0);
            },
            { once: true, capture: true }
          ),
          () => {
            if (backup !== null) clearTimeout(backup);
            backup = null;
          }
        );
        __privateSet(this, _unsubClickListener, unsubClick);
      } else {
        __privateGet(this, _interactOutsideProp).current(event);
      }
    });
    __privateAdd(this, _handleInteractOutside, debounce(__privateGet(this, _interactOutside), 10));
    __privateAdd(this, _markInterceptedEvent, (e) => {
      __privateGet(this, _interceptedEvents)[e.type] = true;
    });
    __privateAdd(this, _markNonInterceptedEvent, (e) => {
      __privateGet(this, _interceptedEvents)[e.type] = false;
    });
    __privateAdd(this, _markResponsibleLayer, (event) => {
      __privateSet(this, _capturedPointerDown, event);
      if (!this.opts.ref.current) return;
      __privateSet(this, _isResponsibleLayer, isResponsibleLayer(this.opts.ref.current));
    });
    __privateAdd(this, _isTargetWithinLayer, (target) => {
      if (!this.opts.ref.current) return false;
      return isOrContainsTarget(this.opts.ref.current, target);
    });
    /**
     * Resets the per-interaction state. Must stay synchronous.
     *
     * This was a `debounce(..., 20)` from when it was also wired to a capture-phase
     * interaction-end listener and had to land after the 10ms `#handleInteractOutside`
     * debounce. That listener is gone, but the debounce stayed on the `cleanup()` path —
     * and because `watch` runs `cleanup()` once per open (`ref` goes null -> node), every
     * layer scheduled a reset 20ms into its own lifetime. An outside `pointerdown` landing
     * 10-20ms after that cleanup would have its `#isResponsibleLayer` flag cleared by the
     * stale reset in the gap before the debounced `#handleInteractOutside` ran, which then
     * bailed and left the layer open. `cleanup()` destroys `#handleInteractOutside` anyway,
     * so nothing is left in flight that needs to observe the pre-reset state.
     */
    __privateAdd(this, _resetState, () => {
      for (const eventType in __privateGet(this, _interceptedEvents)) {
        __privateGet(this, _interceptedEvents)[eventType] = false;
      }
      __privateSet(this, _isResponsibleLayer, false);
      __privateSet(this, _capturedPointerDown, null);
    });
    __privateAdd(this, _onfocuscapture, () => {
      __privateSet(this, _isFocusInsideDOMTree, true);
    });
    __privateAdd(this, _onblurcapture, () => {
      __privateSet(this, _isFocusInsideDOMTree, false);
    });
    __publicField(this, "props", {
      onfocuscapture: __privateGet(this, _onfocuscapture),
      onblurcapture: __privateGet(this, _onblurcapture)
    });
    this.opts = opts;
    __privateSet(this, _behaviorType, opts.interactOutsideBehavior);
    __privateSet(this, _interactOutsideProp, opts.onInteractOutside);
    __privateSet(this, _onFocusOutside, opts.onFocusOutside);
    let unsubEvents = noop;
    let registeredNode = null;
    const cleanup = () => {
      registeredNode = null;
      __privateGet(this, _resetState).call(this);
      globalThis.bitsDismissableLayers.delete(this);
      __privateGet(this, _handleInteractOutside).destroy();
      __privateGet(this, _unsubClickListener).call(this);
      unsubEvents();
    };
    watch([() => this.opts.enabled.current, () => this.opts.ref.current], ([enabled, node]) => {
      const nextNode = enabled ? node : null;
      if (registeredNode === nextNode) return;
      cleanup();
      if (!nextNode) return;
      registeredNode = nextNode;
      __privateSet(this, _documentObj, getOwnerDocument(nextNode));
      globalThis.bitsDismissableLayers.set(this, __privateGet(this, _behaviorType));
      unsubEvents = __privateMethod(this, _DismissibleLayerState_instances, addEventListeners_fn).call(this);
    });
  }
  static create(opts) {
    return new _DismissibleLayerState(opts);
  }
};
_interactOutsideProp = new WeakMap();
_behaviorType = new WeakMap();
_interceptedEvents = new WeakMap();
_isResponsibleLayer = new WeakMap();
_capturedPointerDown = new WeakMap();
_isFocusInsideDOMTree = new WeakMap();
_documentObj = new WeakMap();
_onFocusOutside = new WeakMap();
_unsubClickListener = new WeakMap();
_destroyed = new WeakMap();
_handleFocus = new WeakMap();
_DismissibleLayerState_instances = new WeakSet();
addEventListeners_fn = function() {
  const document2 = __privateGet(this, _documentObj);
  const onPointerDownCapture = (event) => {
    run(() => {
      __privateGet(this, _markInterceptedEvent).call(this, event);
      __privateGet(this, _markResponsibleLayer).call(this, event);
    });
  };
  const onPointerDown = (event) => {
    if (event.cancelBubble || event !== __privateGet(this, _capturedPointerDown)) return;
    __privateGet(this, _markNonInterceptedEvent).call(this, event);
    if (event.pointerType === "touch") {
      __privateGet(this, _handleInteractOutside).destroy();
      __privateGet(this, _interactOutside).call(this, event);
    } else {
      __privateGet(this, _handleInteractOutside).call(this, event);
    }
  };
  document2.addEventListener("pointerdown", onPointerDownCapture, true);
  document2.addEventListener("pointerdown", onPointerDown);
  return executeCallbacks(() => document2.removeEventListener("pointerdown", onPointerDownCapture, true), () => document2.removeEventListener("pointerdown", onPointerDown), on(document2, "focusin", __privateGet(this, _handleFocus)));
};
_handleDismiss = new WeakMap();
_interactOutside = new WeakMap();
_handleInteractOutside = new WeakMap();
_markInterceptedEvent = new WeakMap();
_markNonInterceptedEvent = new WeakMap();
_markResponsibleLayer = new WeakMap();
_isTargetWithinLayer = new WeakMap();
_resetState = new WeakMap();
isAnyEventIntercepted_fn = function() {
  const i = Object.values(__privateGet(this, _interceptedEvents)).some(Boolean);
  return i;
};
_onfocuscapture = new WeakMap();
_onblurcapture = new WeakMap();
let DismissibleLayerState = _DismissibleLayerState;
function getTopMostDismissableLayer(layersArr = [...globalThis.bitsDismissableLayers]) {
  return layersArr.findLast(([_, { current: behaviorType }]) => behaviorType === "close" || behaviorType === "ignore");
}
function isResponsibleLayer(node) {
  const layersArr = [...globalThis.bitsDismissableLayers];
  const topMostLayer = getTopMostDismissableLayer(layersArr);
  if (topMostLayer) return topMostLayer[0].opts.ref.current === node;
  const [firstLayerNode] = layersArr[0];
  return firstLayerNode.opts.ref.current === node;
}
function isValidEvent(e, node) {
  const target = e.target;
  if (!isElementOrSVGElement(target)) return false;
  const targetIsContextMenuTrigger = Boolean(target.closest(`[${CONTEXT_MENU_TRIGGER_ATTR}]`));
  const nodeIsContextMenu = Boolean(node.closest(`[${CONTEXT_MENU_CONTENT_ATTR}]`));
  if ("button" in e && e.button > 0 && !targetIsContextMenuTrigger) return false;
  if ("button" in e && e.button === 0 && targetIsContextMenuTrigger && nodeIsContextMenu) {
    return true;
  }
  if (targetIsContextMenuTrigger && nodeIsContextMenu) return false;
  const ownerDocument = getOwnerDocument(target);
  const isValid = ownerDocument.documentElement.contains(target) && !isOrContainsTarget(node, target) && isClickTrulyOutside(e, node);
  return isValid;
}
function createWrappedEvent(e) {
  const capturedCurrentTarget = e.currentTarget;
  const capturedTarget = e.target;
  let newEvent;
  if (e instanceof PointerEvent) {
    newEvent = new PointerEvent(e.type, e);
  } else {
    newEvent = new PointerEvent("pointerdown", e);
  }
  let isPrevented = false;
  const wrappedEvent = new Proxy(newEvent, {
    get: (target, prop) => {
      if (prop === "currentTarget") {
        return capturedCurrentTarget;
      }
      if (prop === "target") {
        return capturedTarget;
      }
      if (prop === "preventDefault") {
        return () => {
          isPrevented = true;
          if (typeof target.preventDefault === "function") {
            target.preventDefault();
          }
        };
      }
      if (prop === "defaultPrevented") {
        return isPrevented;
      }
      if (prop in target) {
        return target[prop];
      }
      return e[prop];
    }
  });
  return wrappedEvent;
}
function Dismissible_layer($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      interactOutsideBehavior = "close",
      onInteractOutside = noop,
      onFocusOutside = noop,
      id,
      children,
      enabled,
      isValidEvent: isValidEvent2 = () => false,
      ref
    } = $$props;
    const dismissibleLayerState = DismissibleLayerState.create({
      id: boxWith(() => id),
      interactOutsideBehavior: boxWith(() => interactOutsideBehavior),
      onInteractOutside: boxWith(() => onInteractOutside),
      enabled: boxWith(() => enabled),
      onFocusOutside: boxWith(() => onFocusOutside),
      isValidEvent: boxWith(() => isValidEvent2),
      ref
    });
    children == null ? void 0 : children($$renderer2, { props: dismissibleLayerState.props });
    $$renderer2.push(`<!---->`);
  });
}
globalThis.bitsEscapeLayers ?? (globalThis.bitsEscapeLayers = /* @__PURE__ */ new Map());
const _EscapeLayerState = class _EscapeLayerState {
  constructor(opts) {
    __publicField(this, "opts");
    __publicField(this, "domContext");
    __privateAdd(this, _addEventListener, () => {
      return on(this.domContext.getDocument(), "keydown", __privateGet(this, _onkeydown), { passive: false });
    });
    __privateAdd(this, _onkeydown, (e) => {
      if (e.key !== ESCAPE || !isResponsibleEscapeLayer(this)) return;
      const clonedEvent = new KeyboardEvent(e.type, e);
      e.preventDefault();
      const behaviorType = this.opts.escapeKeydownBehavior.current;
      if (behaviorType !== "close" && behaviorType !== "defer-otherwise-close") return;
      this.opts.onEscapeKeydown.current(clonedEvent);
    });
    this.opts = opts;
    this.domContext = new DOMContext(this.opts.ref);
    let unsubEvents = noop;
    watch(() => opts.enabled.current, (enabled) => {
      if (enabled) {
        globalThis.bitsEscapeLayers.set(this, opts.escapeKeydownBehavior);
        unsubEvents = __privateGet(this, _addEventListener).call(this);
      }
      return () => {
        unsubEvents();
        globalThis.bitsEscapeLayers.delete(this);
      };
    });
  }
  static create(opts) {
    return new _EscapeLayerState(opts);
  }
};
_addEventListener = new WeakMap();
_onkeydown = new WeakMap();
let EscapeLayerState = _EscapeLayerState;
function isResponsibleEscapeLayer(instance) {
  const layersArr = [...globalThis.bitsEscapeLayers];
  const topMostLayer = layersArr.findLast(([_, { current: behaviorType }]) => behaviorType === "close" || behaviorType === "ignore");
  if (topMostLayer) return topMostLayer[0] === instance;
  const [firstLayerNode] = layersArr[0];
  return firstLayerNode === instance;
}
function Escape_layer($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      escapeKeydownBehavior = "close",
      onEscapeKeydown = noop,
      children,
      enabled,
      ref
    } = $$props;
    EscapeLayerState.create({
      escapeKeydownBehavior: boxWith(() => escapeKeydownBehavior),
      onEscapeKeydown: boxWith(() => onEscapeKeydown),
      enabled: boxWith(() => enabled),
      ref
    });
    children == null ? void 0 : children($$renderer2);
    $$renderer2.push(`<!---->`);
  });
}
const _FocusScopeManager = class _FocusScopeManager {
  constructor() {
    __privateAdd(this, _scopeStack, simpleBox([]));
    __privateAdd(this, _focusHistory, /* @__PURE__ */ new WeakMap());
    __privateAdd(this, _preFocusHistory, /* @__PURE__ */ new WeakMap());
  }
  static getInstance() {
    if (!this.instance) {
      this.instance = new _FocusScopeManager();
    }
    return this.instance;
  }
  register(scope) {
    const current = this.getActive();
    if (current && current !== scope) {
      current.pause();
    }
    const activeElement = getActiveElement(document);
    if (activeElement && activeElement !== document.body) {
      __privateGet(this, _preFocusHistory).set(scope, activeElement);
    }
    __privateGet(this, _scopeStack).current = __privateGet(this, _scopeStack).current.filter((s) => s !== scope);
    __privateGet(this, _scopeStack).current.unshift(scope);
  }
  unregister(scope) {
    __privateGet(this, _scopeStack).current = __privateGet(this, _scopeStack).current.filter((s) => s !== scope);
    const next = this.getActive();
    if (next) {
      next.resume();
    }
  }
  getActive() {
    return __privateGet(this, _scopeStack).current[0];
  }
  setFocusMemory(scope, element2) {
    __privateGet(this, _focusHistory).set(scope, element2);
  }
  getFocusMemory(scope) {
    return __privateGet(this, _focusHistory).get(scope);
  }
  isActiveScope(scope) {
    return this.getActive() === scope;
  }
  setPreFocusMemory(scope, element2) {
    __privateGet(this, _preFocusHistory).set(scope, element2);
  }
  getPreFocusMemory(scope) {
    return __privateGet(this, _preFocusHistory).get(scope);
  }
  clearPreFocusMemory(scope) {
    __privateGet(this, _preFocusHistory).delete(scope);
  }
};
_scopeStack = new WeakMap();
_focusHistory = new WeakMap();
_preFocusHistory = new WeakMap();
__publicField(_FocusScopeManager, "instance");
let FocusScopeManager = _FocusScopeManager;
/*!
* tabbable 6.5.0
* @license MIT, https://github.com/focus-trap/tabbable/blob/master/LICENSE
*/
var candidateSelectors = ["input:not([inert]):not([inert] *)", "select:not([inert]):not([inert] *)", "textarea:not([inert]):not([inert] *)", "a[href]:not([inert]):not([inert] *)", "area[href]:not([inert]):not([inert] *)", "button:not([inert]):not([inert] *)", "[tabindex]:not(slot):not([inert]):not([inert] *)", "audio[controls]:not([inert]):not([inert] *)", "video[controls]:not([inert]):not([inert] *)", '[contenteditable]:not([contenteditable="false"]):not([inert]):not([inert] *)', "details>summary:first-of-type:not([inert]):not([inert] *)", "details:not([inert]):not([inert] *)"];
var candidateSelector = /* @__PURE__ */ candidateSelectors.join(",");
var NoElement = typeof Element === "undefined";
var matches = NoElement ? function() {
} : Element.prototype.matches || Element.prototype.msMatchesSelector || Element.prototype.webkitMatchesSelector;
var getRootNode = !NoElement && Element.prototype.getRootNode ? function(element2) {
  var _element$getRootNode;
  return element2 === null || element2 === void 0 ? void 0 : (_element$getRootNode = element2.getRootNode) === null || _element$getRootNode === void 0 ? void 0 : _element$getRootNode.call(element2);
} : function(element2) {
  return element2 === null || element2 === void 0 ? void 0 : element2.ownerDocument;
};
var _isInert = function isInert(node, lookUp) {
  var _node$getAttribute;
  if (lookUp === void 0) {
    lookUp = true;
  }
  var inertAtt = node === null || node === void 0 ? void 0 : (_node$getAttribute = node.getAttribute) === null || _node$getAttribute === void 0 ? void 0 : _node$getAttribute.call(node, "inert");
  var inert = inertAtt === "" || inertAtt === "true";
  var result = inert || lookUp && node && // closest does not exist on shadow roots, so we fall back to a manual
  // lookup upward, in case it is not defined.
  (typeof node.closest === "function" ? node.closest("[inert]") : _isInert(node.parentNode));
  return result;
};
var isContentEditable = function isContentEditable2(node) {
  var _node$getAttribute2;
  var attValue = node === null || node === void 0 ? void 0 : (_node$getAttribute2 = node.getAttribute) === null || _node$getAttribute2 === void 0 ? void 0 : _node$getAttribute2.call(node, "contenteditable");
  return attValue === "" || attValue === "true";
};
var getCandidates = function getCandidates2(el, includeContainer, filter) {
  if (_isInert(el)) {
    return [];
  }
  var candidates = Array.prototype.slice.apply(el.querySelectorAll(candidateSelector));
  if (includeContainer && matches.call(el, candidateSelector)) {
    candidates.unshift(el);
  }
  candidates = candidates.filter(filter);
  return candidates;
};
var _getCandidatesIteratively = function getCandidatesIteratively(elements, includeContainer, options) {
  var candidates = [];
  var elementsToCheck = Array.from(elements);
  while (elementsToCheck.length) {
    var element2 = elementsToCheck.shift();
    if (_isInert(element2, false)) {
      continue;
    }
    if (element2.tagName === "SLOT") {
      var assigned = element2.assignedElements();
      var content = assigned.length ? assigned : element2.children;
      var nestedCandidates = _getCandidatesIteratively(content, true, options);
      if (options.flatten) {
        candidates.push.apply(candidates, nestedCandidates);
      } else {
        candidates.push({
          scopeParent: element2,
          candidates: nestedCandidates
        });
      }
    } else {
      var validCandidate = matches.call(element2, candidateSelector);
      if (validCandidate && options.filter(element2) && (includeContainer || !elements.includes(element2))) {
        candidates.push(element2);
      }
      var shadowRoot = element2.shadowRoot || // check for an undisclosed shadow
      typeof options.getShadowRoot === "function" && options.getShadowRoot(element2);
      var validShadowRoot = !_isInert(shadowRoot, false) && (!options.shadowRootFilter || options.shadowRootFilter(element2));
      if (shadowRoot && validShadowRoot) {
        var _nestedCandidates = _getCandidatesIteratively(shadowRoot === true ? element2.children : shadowRoot.children, true, options);
        if (options.flatten) {
          candidates.push.apply(candidates, _nestedCandidates);
        } else {
          candidates.push({
            scopeParent: element2,
            candidates: _nestedCandidates
          });
        }
      } else {
        elementsToCheck.unshift.apply(elementsToCheck, element2.children);
      }
    }
  }
  return candidates;
};
var hasTabIndex = function hasTabIndex2(node) {
  return !isNaN(parseInt(node.getAttribute("tabindex"), 10));
};
var getTabIndex = function getTabIndex2(node) {
  if (!node) {
    throw new Error("No node provided");
  }
  if (node.tabIndex < 0) {
    if ((/^(AUDIO|VIDEO|DETAILS)$/.test(node.tagName) || isContentEditable(node)) && !hasTabIndex(node)) {
      return 0;
    }
  }
  return node.tabIndex;
};
var getSortOrderTabIndex = function getSortOrderTabIndex2(node, isScope) {
  var tabIndex = getTabIndex(node);
  if (tabIndex < 0 && isScope && !hasTabIndex(node)) {
    return 0;
  }
  return tabIndex;
};
var sortOrderedTabbables = function sortOrderedTabbables2(a, b) {
  return a.tabIndex === b.tabIndex ? a.documentOrder - b.documentOrder : a.tabIndex - b.tabIndex;
};
var isInput = function isInput2(node) {
  return node.tagName === "INPUT";
};
var isHiddenInput = function isHiddenInput2(node) {
  return isInput(node) && node.type === "hidden";
};
var isDetailsWithSummary = function isDetailsWithSummary2(node) {
  var r2 = node.tagName === "DETAILS" && Array.prototype.slice.apply(node.children).some(function(child) {
    return child.tagName === "SUMMARY";
  });
  return r2;
};
var getCheckedRadio = function getCheckedRadio2(nodes, form) {
  for (var i = 0; i < nodes.length; i++) {
    if (nodes[i].checked && nodes[i].form === form) {
      return nodes[i];
    }
  }
};
var isTabbableRadio = function isTabbableRadio2(node) {
  if (!node.name) {
    return true;
  }
  var radioScope = node.form || getRootNode(node);
  var queryRadios = function queryRadios2(name) {
    return radioScope.querySelectorAll('input[type="radio"][name="' + name + '"]');
  };
  var radioSet;
  if (typeof window !== "undefined" && typeof window.CSS !== "undefined" && typeof window.CSS.escape === "function") {
    radioSet = queryRadios(window.CSS.escape(node.name));
  } else {
    try {
      radioSet = queryRadios(node.name);
    } catch (err) {
      console.error("Looks like you have a radio button with a name attribute containing invalid CSS selector characters and need the CSS.escape polyfill: %s", err.message);
      return false;
    }
  }
  var checked = getCheckedRadio(radioSet, node.form);
  return !checked || checked === node;
};
var isRadio = function isRadio2(node) {
  return isInput(node) && node.type === "radio";
};
var isNonTabbableRadio = function isNonTabbableRadio2(node) {
  return isRadio(node) && !isTabbableRadio(node);
};
var isNodeAttached = function isNodeAttached2(node) {
  var _nodeRoot;
  var nodeRoot = node && getRootNode(node);
  var nodeRootHost = (_nodeRoot = nodeRoot) === null || _nodeRoot === void 0 ? void 0 : _nodeRoot.host;
  var attached = false;
  if (nodeRoot && nodeRoot !== node) {
    var _nodeRootHost, _nodeRootHost$ownerDo, _node$ownerDocument;
    attached = !!((_nodeRootHost = nodeRootHost) !== null && _nodeRootHost !== void 0 && (_nodeRootHost$ownerDo = _nodeRootHost.ownerDocument) !== null && _nodeRootHost$ownerDo !== void 0 && _nodeRootHost$ownerDo.contains(nodeRootHost) || node !== null && node !== void 0 && (_node$ownerDocument = node.ownerDocument) !== null && _node$ownerDocument !== void 0 && _node$ownerDocument.contains(node));
    while (!attached && nodeRootHost) {
      var _nodeRoot2, _nodeRootHost2, _nodeRootHost2$ownerD;
      nodeRoot = getRootNode(nodeRootHost);
      nodeRootHost = (_nodeRoot2 = nodeRoot) === null || _nodeRoot2 === void 0 ? void 0 : _nodeRoot2.host;
      attached = !!((_nodeRootHost2 = nodeRootHost) !== null && _nodeRootHost2 !== void 0 && (_nodeRootHost2$ownerD = _nodeRootHost2.ownerDocument) !== null && _nodeRootHost2$ownerD !== void 0 && _nodeRootHost2$ownerD.contains(nodeRootHost));
    }
  }
  return attached;
};
var isZeroArea = function isZeroArea2(node) {
  var _node$getBoundingClie = node.getBoundingClientRect(), width = _node$getBoundingClie.width, height = _node$getBoundingClie.height;
  return width === 0 && height === 0;
};
var isHidden = function isHidden2(node, _ref) {
  var displayCheck = _ref.displayCheck, getShadowRoot = _ref.getShadowRoot;
  if (displayCheck === "full-native") {
    if ("checkVisibility" in node) {
      var visible = node.checkVisibility({
        // Checking opacity might be desirable for some use cases, but natively,
        // opacity zero elements _are_ focusable and tabbable.
        checkOpacity: false,
        opacityProperty: false,
        contentVisibilityAuto: true,
        visibilityProperty: true,
        // This is an alias for `visibilityProperty`. Contemporary browsers
        // support both. However, this alias has wider browser support (Chrome
        // >= 105 and Firefox >= 106, vs. Chrome >= 121 and Firefox >= 122), so
        // we include it anyway.
        checkVisibilityCSS: true
      });
      return !visible;
    }
  }
  var _getComputedStyle = getComputedStyle(node), visibility = _getComputedStyle.visibility;
  if (visibility === "hidden" || visibility === "collapse") {
    return true;
  }
  var isDirectSummary = matches.call(node, "details>summary:first-of-type");
  var nodeUnderDetails = isDirectSummary ? node.parentElement : node;
  if (matches.call(nodeUnderDetails, "details:not([open]) *")) {
    return true;
  }
  if (!displayCheck || displayCheck === "full" || // full-native can run this branch when it falls through in case
  // Element#checkVisibility is unsupported
  displayCheck === "full-native" || displayCheck === "legacy-full") {
    if (typeof getShadowRoot === "function") {
      var originalNode = node;
      while (node) {
        var parentElement = node.parentElement;
        var rootNode = getRootNode(node);
        if (parentElement && !parentElement.shadowRoot && getShadowRoot(parentElement) === true) {
          return isZeroArea(node);
        } else if (node.assignedSlot) {
          node = node.assignedSlot;
        } else if (!parentElement && rootNode !== node.ownerDocument) {
          node = rootNode.host;
        } else {
          node = parentElement;
        }
      }
      node = originalNode;
    }
    if (isNodeAttached(node)) {
      return !node.getClientRects().length;
    }
    if (displayCheck !== "legacy-full") {
      return true;
    }
  } else if (displayCheck === "non-zero-area") {
    return isZeroArea(node);
  }
  return false;
};
var isDisabledFromFieldset = function isDisabledFromFieldset2(node) {
  if (/^(INPUT|BUTTON|SELECT|TEXTAREA)$/.test(node.tagName)) {
    var parentNode = node.parentElement;
    while (parentNode) {
      if (parentNode.tagName === "FIELDSET" && parentNode.disabled) {
        for (var i = 0; i < parentNode.children.length; i++) {
          var child = parentNode.children.item(i);
          if (child.tagName === "LEGEND") {
            return matches.call(parentNode, "fieldset[disabled] *") ? true : !child.contains(node);
          }
        }
        return true;
      }
      parentNode = parentNode.parentElement;
    }
  }
  return false;
};
var isNodeMatchingSelectorFocusable = function isNodeMatchingSelectorFocusable2(options, node) {
  if (node.disabled || isHiddenInput(node) || isHidden(node, options) || // For a details element with a summary, the summary element gets the focus
  isDetailsWithSummary(node) || isDisabledFromFieldset(node)) {
    return false;
  }
  return true;
};
var isNodeMatchingSelectorTabbable = function isNodeMatchingSelectorTabbable2(options, node) {
  if (isNonTabbableRadio(node) || getTabIndex(node) < 0 || !isNodeMatchingSelectorFocusable(options, node)) {
    return false;
  }
  return true;
};
var isShadowRootTabbable = function isShadowRootTabbable2(shadowHostNode) {
  var tabIndex = parseInt(shadowHostNode.getAttribute("tabindex"), 10);
  if (isNaN(tabIndex) || tabIndex >= 0) {
    return true;
  }
  return false;
};
var _sortByOrder = function sortByOrder(candidates) {
  var regularTabbables = [];
  var orderedTabbables = [];
  candidates.forEach(function(item, i) {
    var isScope = !!item.scopeParent;
    var element2 = isScope ? item.scopeParent : item;
    var candidateTabindex = getSortOrderTabIndex(element2, isScope);
    var elements = isScope ? _sortByOrder(item.candidates) : element2;
    if (candidateTabindex === 0) {
      isScope ? regularTabbables.push.apply(regularTabbables, elements) : regularTabbables.push(element2);
    } else {
      orderedTabbables.push({
        documentOrder: i,
        tabIndex: candidateTabindex,
        item,
        isScope,
        content: elements
      });
    }
  });
  return orderedTabbables.sort(sortOrderedTabbables).reduce(function(acc, sortable) {
    sortable.isScope ? acc.push.apply(acc, sortable.content) : acc.push(sortable.content);
    return acc;
  }, []).concat(regularTabbables);
};
var tabbable = function tabbable2(container, options) {
  options = options || {};
  var candidates;
  if (options.getShadowRoot) {
    candidates = _getCandidatesIteratively([container], options.includeContainer, {
      filter: isNodeMatchingSelectorTabbable.bind(null, options),
      flatten: false,
      getShadowRoot: options.getShadowRoot,
      shadowRootFilter: isShadowRootTabbable
    });
  } else {
    candidates = getCandidates(container, options.includeContainer, isNodeMatchingSelectorTabbable.bind(null, options));
  }
  return _sortByOrder(candidates);
};
var focusable = function focusable2(container, options) {
  options = options || {};
  var candidates;
  if (options.getShadowRoot) {
    candidates = _getCandidatesIteratively([container], options.includeContainer, {
      filter: isNodeMatchingSelectorFocusable.bind(null, options),
      flatten: true,
      getShadowRoot: options.getShadowRoot
    });
  } else {
    candidates = getCandidates(container, options.includeContainer, isNodeMatchingSelectorFocusable.bind(null, options));
  }
  return candidates;
};
var isTabbable = function isTabbable2(node, options) {
  options = options || {};
  if (!node) {
    throw new Error("No node provided");
  }
  if (matches.call(node, candidateSelector) === false) {
    return false;
  }
  return isNodeMatchingSelectorTabbable(options, node);
};
var focusableCandidateSelector = /* @__PURE__ */ candidateSelectors.concat("iframe:not([inert]):not([inert] *)").join(",");
var isFocusable = function isFocusable2(node, options) {
  options = options || {};
  if (!node) {
    throw new Error("No node provided");
  }
  if (matches.call(node, focusableCandidateSelector) === false) {
    return false;
  }
  return isNodeMatchingSelectorFocusable(options, node);
};
const _FocusScope = class _FocusScope {
  constructor(opts) {
    __privateAdd(this, _FocusScope_instances);
    __privateAdd(this, _paused, false);
    __privateAdd(this, _container, null);
    __privateAdd(this, _manager, FocusScopeManager.getInstance());
    __privateAdd(this, _cleanupFns, []);
    __privateAdd(this, _opts3);
    __privateSet(this, _opts3, opts);
  }
  get paused() {
    return __privateGet(this, _paused);
  }
  pause() {
    __privateSet(this, _paused, true);
  }
  resume() {
    __privateSet(this, _paused, false);
  }
  mount(container) {
    if (__privateGet(this, _container)) {
      this.unmount();
    }
    __privateSet(this, _container, container);
    __privateGet(this, _manager).register(this);
    __privateMethod(this, _FocusScope_instances, setupEventListeners_fn).call(this);
    __privateMethod(this, _FocusScope_instances, handleOpenAutoFocus_fn).call(this);
  }
  unmount() {
    if (!__privateGet(this, _container)) return;
    __privateMethod(this, _FocusScope_instances, cleanup_fn2).call(this);
    __privateMethod(this, _FocusScope_instances, handleCloseAutoFocus_fn).call(this);
    __privateGet(this, _manager).unregister(this);
    __privateGet(this, _manager).clearPreFocusMemory(this);
    __privateSet(this, _container, null);
  }
  static use(opts) {
    let scope = null;
    watch([() => opts.ref.current, () => opts.enabled.current], ([ref, enabled]) => {
      if (ref && enabled) {
        if (!scope) {
          scope = new _FocusScope(opts);
        }
        scope.mount(ref);
      } else if (scope) {
        scope.unmount();
        scope = null;
      }
    });
    return {
      get props() {
        return { tabindex: -1 };
      }
    };
  }
};
_paused = new WeakMap();
_container = new WeakMap();
_manager = new WeakMap();
_cleanupFns = new WeakMap();
_opts3 = new WeakMap();
_FocusScope_instances = new WeakSet();
cleanup_fn2 = function() {
  for (const fn of __privateGet(this, _cleanupFns)) {
    fn();
  }
  __privateSet(this, _cleanupFns, []);
};
handleOpenAutoFocus_fn = function() {
  if (!__privateGet(this, _container)) return;
  const event = new CustomEvent("focusScope.onOpenAutoFocus", { bubbles: false, cancelable: true });
  __privateGet(this, _opts3).onOpenAutoFocus.current(event);
  if (!event.defaultPrevented) {
    requestAnimationFrame(() => {
      if (!__privateGet(this, _container) || __privateGet(this, _paused) || !__privateGet(this, _manager).isActiveScope(this) || __privateGet(this, _container).contains(getActiveElement(__privateGet(this, _container).ownerDocument))) {
        return;
      }
      const firstTabbable = __privateMethod(this, _FocusScope_instances, getFirstTabbable_fn).call(this);
      if (firstTabbable) {
        firstTabbable.focus();
        __privateGet(this, _manager).setFocusMemory(this, firstTabbable);
      } else {
        __privateGet(this, _container).focus();
      }
    });
  }
};
handleCloseAutoFocus_fn = function() {
  var _a, _b;
  const event = new CustomEvent("focusScope.onCloseAutoFocus", { bubbles: false, cancelable: true });
  (_b = (_a = __privateGet(this, _opts3).onCloseAutoFocus).current) == null ? void 0 : _b.call(_a, event);
  if (!event.defaultPrevented) {
    const preFocusedElement = __privateGet(this, _manager).getPreFocusMemory(this);
    if (preFocusedElement && preFocusedElement.isConnected) {
      try {
        preFocusedElement.focus();
      } catch {
        document.body.focus();
      }
    }
  }
};
setupEventListeners_fn = function() {
  if (!__privateGet(this, _container) || !__privateGet(this, _opts3).trap.current) return;
  const container = __privateGet(this, _container);
  const doc = container.ownerDocument;
  const handleFocus = (e) => {
    if (__privateGet(this, _paused) || !__privateGet(this, _manager).isActiveScope(this)) return;
    const target = e.composedPath()[0];
    if (!target) return;
    const isInside = container.contains(target);
    if (isInside) {
      __privateGet(this, _manager).setFocusMemory(this, target);
    } else {
      const lastFocused = __privateGet(this, _manager).getFocusMemory(this);
      if (lastFocused && container.contains(lastFocused) && isFocusable(lastFocused)) {
        e.preventDefault();
        lastFocused.focus();
      } else {
        const firstTabbable = __privateMethod(this, _FocusScope_instances, getFirstTabbable_fn).call(this);
        const firstFocusable = __privateMethod(this, _FocusScope_instances, getAllFocusables_fn).call(this)[0];
        (firstTabbable || firstFocusable || container).focus();
      }
    }
  };
  const handleKeydown = (e) => {
    if (!__privateGet(this, _opts3).loop || __privateGet(this, _paused) || e.key !== "Tab") return;
    if (!__privateGet(this, _manager).isActiveScope(this)) return;
    const tabbables = __privateMethod(this, _FocusScope_instances, getTabbables_fn).call(this);
    if (tabbables.length === 0) return;
    const first = tabbables[0];
    const last = tabbables[tabbables.length - 1];
    if (!e.shiftKey && getActiveElement(doc) === last) {
      e.preventDefault();
      first.focus();
    } else if (e.shiftKey && getActiveElement(doc) === first) {
      e.preventDefault();
      last.focus();
    }
  };
  __privateGet(this, _cleanupFns).push(on(doc, "focusin", handleFocus, { capture: true }), on(container, "keydown", handleKeydown));
  const observer = new MutationObserver(() => {
    const lastFocused = __privateGet(this, _manager).getFocusMemory(this);
    if (lastFocused && !container.contains(lastFocused)) {
      const firstTabbable = __privateMethod(this, _FocusScope_instances, getFirstTabbable_fn).call(this);
      const firstFocusable = __privateMethod(this, _FocusScope_instances, getAllFocusables_fn).call(this)[0];
      const elementToFocus = firstTabbable || firstFocusable;
      if (elementToFocus) {
        elementToFocus.focus();
        __privateGet(this, _manager).setFocusMemory(this, elementToFocus);
      } else {
        container.focus();
      }
    }
  });
  observer.observe(container, { childList: true, subtree: true });
  __privateGet(this, _cleanupFns).push(() => observer.disconnect());
};
getTabbables_fn = function() {
  if (!__privateGet(this, _container)) return [];
  return tabbable(__privateGet(this, _container), { includeContainer: false, getShadowRoot: true });
};
getFirstTabbable_fn = function() {
  const tabbables = __privateMethod(this, _FocusScope_instances, getTabbables_fn).call(this);
  return tabbables[0] || null;
};
getAllFocusables_fn = function() {
  if (!__privateGet(this, _container)) return [];
  return focusable(__privateGet(this, _container), { includeContainer: false, getShadowRoot: true });
};
let FocusScope = _FocusScope;
function Focus_scope($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      enabled = false,
      trapFocus = false,
      loop = false,
      onCloseAutoFocus = noop,
      onOpenAutoFocus = noop,
      focusScope,
      ref
    } = $$props;
    const focusScopeState = FocusScope.use({
      enabled: boxWith(() => enabled),
      trap: boxWith(() => trapFocus),
      loop,
      onCloseAutoFocus: boxWith(() => onCloseAutoFocus),
      onOpenAutoFocus: boxWith(() => onOpenAutoFocus),
      ref
    });
    focusScope == null ? void 0 : focusScope($$renderer2, { props: focusScopeState.props });
    $$renderer2.push(`<!---->`);
  });
}
const noopPointer = () => {
};
globalThis.bitsTextSelectionLayers ?? (globalThis.bitsTextSelectionLayers = /* @__PURE__ */ new Map());
const _TextSelectionLayerState = class _TextSelectionLayerState {
  constructor(opts) {
    __privateAdd(this, _TextSelectionLayerState_instances);
    __publicField(this, "opts");
    __publicField(this, "domContext");
    __privateAdd(this, _unsubSelectionLock, noop);
    __privateAdd(this, _enabledSnapshot, false);
    __privateAdd(this, _onPointerDownSnapshot, noopPointer);
    __privateAdd(this, _onPointerUpSnapshot, noopPointer);
    __privateAdd(this, _pointerup, (e) => {
      __privateGet(this, _resetSelectionLock).call(this);
      if (e.defaultPrevented) return;
      __privateGet(this, _onPointerUpSnapshot).call(this, e);
    });
    __privateAdd(this, _pointerdown, (e) => {
      if (!__privateGet(this, _enabledSnapshot)) return;
      __privateGet(this, _resetSelectionLock).call(this);
      const node = this.opts.ref.current;
      const target = e.target;
      if (!isHTMLElement$1(node) || !isHTMLElement$1(target)) return;
      if (!isHighestLayer(this) || !contains(node, target)) return;
      __privateGet(this, _onPointerDownSnapshot).call(this, e);
      if (e.defaultPrevented) return;
      __privateSet(this, _unsubSelectionLock, preventTextSelectionOverflow(node, this.domContext.getDocument().body));
    });
    __privateAdd(this, _resetSelectionLock, () => {
      __privateGet(this, _unsubSelectionLock).call(this);
      __privateSet(this, _unsubSelectionLock, noop);
    });
    this.opts = opts;
    this.domContext = new DOMContext(opts.ref);
    let unsubEvents = noop;
    watch(
      () => [
        this.opts.enabled.current,
        this.opts.onPointerDown.current,
        this.opts.onPointerUp.current
      ],
      ([enabled, onPointerDown, onPointerUp]) => {
        __privateSet(this, _enabledSnapshot, enabled);
        __privateSet(this, _onPointerDownSnapshot, onPointerDown);
        __privateSet(this, _onPointerUpSnapshot, onPointerUp);
        if (enabled) {
          globalThis.bitsTextSelectionLayers.set(this, this.opts.enabled);
          unsubEvents();
          unsubEvents = __privateMethod(this, _TextSelectionLayerState_instances, addEventListeners_fn2).call(this);
        }
        return () => {
          __privateSet(this, _enabledSnapshot, false);
          unsubEvents();
          __privateGet(this, _resetSelectionLock).call(this);
          globalThis.bitsTextSelectionLayers.delete(this);
        };
      }
    );
  }
  static create(opts) {
    return new _TextSelectionLayerState(opts);
  }
};
_unsubSelectionLock = new WeakMap();
_enabledSnapshot = new WeakMap();
_onPointerDownSnapshot = new WeakMap();
_onPointerUpSnapshot = new WeakMap();
_TextSelectionLayerState_instances = new WeakSet();
addEventListeners_fn2 = function() {
  return executeCallbacks(on(this.domContext.getDocument(), "pointerdown", __privateGet(this, _pointerdown)), on(this.domContext.getDocument(), "pointerup", __privateGet(this, _pointerup)));
};
_pointerup = new WeakMap();
_pointerdown = new WeakMap();
_resetSelectionLock = new WeakMap();
let TextSelectionLayerState = _TextSelectionLayerState;
const getUserSelect = (node) => node.style.userSelect || node.style.webkitUserSelect;
function preventTextSelectionOverflow(node, body) {
  const originalBodyUserSelect = getUserSelect(body);
  const originalNodeUserSelect = getUserSelect(node);
  setUserSelect(body, "none");
  setUserSelect(node, "text");
  return () => {
    setUserSelect(body, originalBodyUserSelect);
    setUserSelect(node, originalNodeUserSelect);
  };
}
function setUserSelect(node, value) {
  node.style.userSelect = value;
  node.style.webkitUserSelect = value;
}
function isHighestLayer(instance) {
  const layersArr = [...globalThis.bitsTextSelectionLayers];
  if (!layersArr.length) return false;
  const highestLayer = layersArr.at(-1);
  if (!highestLayer) return false;
  return highestLayer[0] === instance;
}
function Text_selection_layer($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      preventOverflowTextSelection = true,
      onPointerDown = noop,
      onPointerUp = noop,
      id,
      children,
      enabled,
      ref
    } = $$props;
    TextSelectionLayerState.create({
      id: boxWith(() => id),
      onPointerDown: boxWith(() => onPointerDown),
      onPointerUp: boxWith(() => onPointerUp),
      enabled: boxWith(() => enabled && preventOverflowTextSelection),
      ref
    });
    children == null ? void 0 : children($$renderer2);
    $$renderer2.push(`<!---->`);
  });
}
globalThis.bitsIdCounter ?? (globalThis.bitsIdCounter = { current: 0 });
function useId(prefix = "bits") {
  globalThis.bitsIdCounter.current++;
  return `${prefix}-${globalThis.bitsIdCounter.current}`;
}
class SharedState {
  constructor(factory) {
    __privateAdd(this, _SharedState_instances);
    __privateAdd(this, _factory);
    __privateAdd(this, _subscribers, 0);
    __privateAdd(this, _state);
    __privateAdd(this, _scope);
    __privateSet(this, _factory, factory);
  }
  get(...args) {
    __privateSet(this, _subscribers, __privateGet(this, _subscribers) + 1);
    if (__privateGet(this, _state) === void 0) {
      __privateSet(this, _scope, () => {
      });
    }
    return __privateGet(this, _state);
  }
}
_factory = new WeakMap();
_subscribers = new WeakMap();
_state = new WeakMap();
_scope = new WeakMap();
_SharedState_instances = new WeakSet();
dispose_fn = function() {
  __privateSet(this, _subscribers, __privateGet(this, _subscribers) - 1);
  if (__privateGet(this, _scope) && __privateGet(this, _subscribers) <= 0) {
    __privateGet(this, _scope).call(this);
    __privateSet(this, _state, void 0);
    __privateSet(this, _scope, void 0);
  }
};
const lockMap = new SvelteMap();
const MANAGED_PROPERTIES = [
  "padding-right",
  "margin-right",
  "overflow",
  "overflow-x",
  "overflow-y",
  "pointer-events",
  "--scrollbar-width"
];
let initialProperties = null;
const modifiedProperties = /* @__PURE__ */ new Set();
let cleanupTimeoutId = null;
let isInCleanupTransition = false;
function setManagedProperty(style, property, value) {
  style.setProperty(property, value);
  modifiedProperties.add(property);
  if (property === "overflow") {
    modifiedProperties.add("overflow-x");
    modifiedProperties.add("overflow-y");
  }
}
const anyLocked = boxWith(() => {
  for (const value of lockMap.values()) {
    if (value) return true;
  }
  return false;
});
let cleanupScheduledAt = null;
const bodyLockStackCount = new SharedState(() => {
  function resetBodyStyle(documentObj) {
    return;
  }
  function cancelPendingCleanup() {
    if (cleanupTimeoutId === null) return;
    window.clearTimeout(cleanupTimeoutId);
    cleanupTimeoutId = null;
  }
  function scheduleCleanupIfNoNewLocks(delay, callback) {
    cancelPendingCleanup();
    isInCleanupTransition = true;
    cleanupScheduledAt = Date.now();
    const currentCleanupId = cleanupScheduledAt;
    const cleanupFn = () => {
      cleanupTimeoutId = null;
      if (cleanupScheduledAt !== currentCleanupId) return;
      if (!isAnyLocked(lockMap)) {
        isInCleanupTransition = false;
        callback();
      } else {
        isInCleanupTransition = false;
      }
    };
    const actualDelay = delay === null ? 24 : delay;
    cleanupTimeoutId = window.setTimeout(cleanupFn, actualDelay);
  }
  function ensureInitialStyleCaptured() {
    if (initialProperties === null && lockMap.size === 0 && !isInCleanupTransition) {
      document.body.hasAttribute("style");
      initialProperties = /* @__PURE__ */ new Map();
      for (const prop of MANAGED_PROPERTIES) {
        initialProperties.set(prop, {
          value: document.body.style.getPropertyValue(prop),
          priority: document.body.style.getPropertyPriority(prop)
        });
      }
    }
  }
  watch(() => anyLocked.current, () => {
    var _a, _b;
    if (!anyLocked.current) return;
    ensureInitialStyleCaptured();
    isInCleanupTransition = false;
    const htmlStyle = getComputedStyle(document.documentElement);
    const bodyStyle = getComputedStyle(document.body);
    const hasStableGutter = ((_a = htmlStyle.scrollbarGutter) == null ? void 0 : _a.includes("stable")) || ((_b = bodyStyle.scrollbarGutter) == null ? void 0 : _b.includes("stable"));
    const verticalScrollbarWidth = window.innerWidth - document.documentElement.clientWidth;
    const paddingRight = Number.parseInt(bodyStyle.paddingRight ?? "0", 10);
    const config = {
      padding: paddingRight + verticalScrollbarWidth,
      margin: Number.parseInt(bodyStyle.marginRight ?? "0", 10)
    };
    if (verticalScrollbarWidth > 0 && !hasStableGutter) {
      setManagedProperty(document.body.style, "padding-right", `${config.padding}px`);
      setManagedProperty(document.body.style, "margin-right", `${config.margin}px`);
      setManagedProperty(document.body.style, "--scrollbar-width", `${verticalScrollbarWidth}px`);
    }
    setManagedProperty(document.body.style, "overflow", "hidden");
    if (isIOS) {
      on(
        document,
        "touchmove",
        (e) => {
          if (e.target !== document.documentElement) return;
          if (e.touches.length > 1) return;
          e.preventDefault();
        },
        { passive: false }
      );
    }
    afterTick(() => {
      setManagedProperty(document.body.style, "pointer-events", "none");
      setManagedProperty(document.body.style, "overflow", "hidden");
    });
  });
  return {
    get lockMap() {
      return lockMap;
    },
    resetBodyStyle,
    scheduleCleanupIfNoNewLocks,
    cancelPendingCleanup,
    ensureInitialStyleCaptured
  };
});
class BodyScrollLock {
  constructor(initialState, restoreScrollDelay = () => null) {
    __privateAdd(this, _id, useId());
    __privateAdd(this, _initialState);
    __privateAdd(this, _restoreScrollDelay, () => null);
    __privateAdd(this, _countState);
    __publicField(this, "locked");
    __privateSet(this, _initialState, initialState);
    __privateSet(this, _restoreScrollDelay, restoreScrollDelay);
    __privateSet(this, _countState, bodyLockStackCount.get());
    if (!__privateGet(this, _countState)) return;
    __privateGet(this, _countState).cancelPendingCleanup();
    __privateGet(this, _countState).ensureInitialStyleCaptured();
    __privateGet(this, _countState).lockMap.set(__privateGet(this, _id), __privateGet(this, _initialState) ?? false);
    this.locked = boxWith(() => __privateGet(this, _countState).lockMap.get(__privateGet(this, _id)) ?? false, (v) => __privateGet(this, _countState).lockMap.set(__privateGet(this, _id), v));
  }
}
_id = new WeakMap();
_initialState = new WeakMap();
_restoreScrollDelay = new WeakMap();
_countState = new WeakMap();
function isAnyLocked(map) {
  for (const [_, value] of map) {
    if (value) return true;
  }
  return false;
}
function Scroll_lock($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let { preventScroll = true, restoreScrollDelay = null } = $$props;
    if (preventScroll) {
      new BodyScrollLock(preventScroll, () => restoreScrollDelay);
    }
  });
}
const FloatingRootContext = new Context("Floating.Root");
const FloatingTooltipRootContext = new Context("Floating.Root");
class FloatingRootState {
  constructor() {
    __publicField(this, "anchorNode", simpleBox(null));
    __publicField(this, "customAnchorNode", simpleBox(null));
    __publicField(this, "triggerNode", simpleBox(null));
  }
  static create(tooltip = false) {
    return tooltip ? FloatingTooltipRootContext.set(new FloatingRootState()) : FloatingRootContext.set(new FloatingRootState());
  }
}
class FloatingAnchorState {
  constructor(opts, root) {
    __publicField(this, "opts");
    __publicField(this, "root");
    this.opts = opts;
    this.root = root;
    if (opts.virtualEl && opts.virtualEl.current) {
      root.triggerNode = boxFrom(opts.virtualEl.current);
    } else {
      root.triggerNode = opts.ref;
    }
  }
  static create(opts, tooltip = false) {
    return tooltip ? new FloatingAnchorState(opts, FloatingTooltipRootContext.get()) : new FloatingAnchorState(opts, FloatingRootContext.get());
  }
}
function Floating_layer($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let { children, tooltip = false } = $$props;
    FloatingRootState.create(tooltip);
    children == null ? void 0 : children($$renderer2);
    $$renderer2.push(`<!---->`);
  });
}
function get(valueOrGetValue) {
  return typeof valueOrGetValue === "function" ? valueOrGetValue() : valueOrGetValue;
}
function getDPR(element2) {
  if (typeof window === "undefined") return 1;
  const win = element2.ownerDocument.defaultView || window;
  return win.devicePixelRatio || 1;
}
function roundByDPR(element2, value) {
  const dpr = getDPR(element2);
  return Math.round(value * dpr) / dpr;
}
function getFloatingContentCSSVars(name) {
  return {
    [`--bits-${name}-content-transform-origin`]: `var(--bits-floating-transform-origin)`,
    [`--bits-${name}-content-available-width`]: `var(--bits-floating-available-width)`,
    [`--bits-${name}-content-available-height`]: `var(--bits-floating-available-height)`,
    [`--bits-${name}-anchor-width`]: `var(--bits-floating-anchor-width)`,
    [`--bits-${name}-anchor-height`]: `var(--bits-floating-anchor-height)`
  };
}
function Floating_layer_anchor($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let { id, children, virtualEl, ref, tooltip = false } = $$props;
    FloatingAnchorState.create(
      {
        id: boxWith(() => id),
        virtualEl: boxWith(() => virtualEl),
        ref
      },
      tooltip
    );
    children == null ? void 0 : children($$renderer2);
    $$renderer2.push(`<!---->`);
  });
}
const sides = ["top", "right", "bottom", "left"];
const min = Math.min;
const max = Math.max;
const round = Math.round;
const createCoords = (v) => ({
  x: v,
  y: v
});
const oppositeSideMap = {
  left: "right",
  right: "left",
  bottom: "top",
  top: "bottom"
};
function clamp(start, value, end) {
  return max(start, min(value, end));
}
function evaluate(value, param) {
  return typeof value === "function" ? value(param) : value;
}
function getSide$1(placement) {
  return placement.split("-")[0];
}
function getAlignment(placement) {
  return placement.split("-")[1];
}
function getOppositeAxis(axis) {
  return axis === "x" ? "y" : "x";
}
function getAxisLength(axis) {
  return axis === "y" ? "height" : "width";
}
function getSideAxis(placement) {
  const firstChar = placement[0];
  return firstChar === "t" || firstChar === "b" ? "y" : "x";
}
function getAlignmentAxis(placement) {
  return getOppositeAxis(getSideAxis(placement));
}
function getAlignmentSides(placement, rects, rtl) {
  if (rtl === void 0) {
    rtl = false;
  }
  const alignment = getAlignment(placement);
  const alignmentAxis = getAlignmentAxis(placement);
  const length = getAxisLength(alignmentAxis);
  let mainAlignmentSide = alignmentAxis === "x" ? alignment === (rtl ? "end" : "start") ? "right" : "left" : alignment === "start" ? "bottom" : "top";
  if (rects.reference[length] > rects.floating[length]) {
    mainAlignmentSide = getOppositePlacement(mainAlignmentSide);
  }
  return [mainAlignmentSide, getOppositePlacement(mainAlignmentSide)];
}
function getExpandedPlacements(placement) {
  const oppositePlacement = getOppositePlacement(placement);
  return [getOppositeAlignmentPlacement(placement), oppositePlacement, getOppositeAlignmentPlacement(oppositePlacement)];
}
function getOppositeAlignmentPlacement(placement) {
  return placement.includes("start") ? placement.replace("start", "end") : placement.replace("end", "start");
}
const lrPlacement = ["left", "right"];
const rlPlacement = ["right", "left"];
const tbPlacement = ["top", "bottom"];
const btPlacement = ["bottom", "top"];
function getSideList(side, isStart, rtl) {
  switch (side) {
    case "top":
    case "bottom":
      if (rtl) return isStart ? rlPlacement : lrPlacement;
      return isStart ? lrPlacement : rlPlacement;
    case "left":
    case "right":
      return isStart ? tbPlacement : btPlacement;
    default:
      return [];
  }
}
function getOppositeAxisPlacements(placement, flipAlignment, direction, rtl) {
  const alignment = getAlignment(placement);
  let list = getSideList(getSide$1(placement), direction === "start", rtl);
  if (alignment) {
    list = list.map((side) => side + "-" + alignment);
    if (flipAlignment) {
      list = list.concat(list.map(getOppositeAlignmentPlacement));
    }
  }
  return list;
}
function getOppositePlacement(placement) {
  const side = getSide$1(placement);
  return oppositeSideMap[side] + placement.slice(side.length);
}
function expandPaddingObject(padding) {
  var _padding$top, _padding$right, _padding$bottom, _padding$left;
  return {
    top: (_padding$top = padding.top) != null ? _padding$top : 0,
    right: (_padding$right = padding.right) != null ? _padding$right : 0,
    bottom: (_padding$bottom = padding.bottom) != null ? _padding$bottom : 0,
    left: (_padding$left = padding.left) != null ? _padding$left : 0
  };
}
function getPaddingObject(padding) {
  return typeof padding !== "number" ? expandPaddingObject(padding) : {
    top: padding,
    right: padding,
    bottom: padding,
    left: padding
  };
}
function rectToClientRect(rect) {
  const {
    x,
    y,
    width,
    height
  } = rect;
  return {
    width,
    height,
    top: y,
    left: x,
    right: x + width,
    bottom: y + height,
    x,
    y
  };
}
function computeCoordsFromPlacement(_ref, placement, rtl) {
  let {
    reference,
    floating
  } = _ref;
  const sideAxis = getSideAxis(placement);
  const alignmentAxis = getAlignmentAxis(placement);
  const alignLength = getAxisLength(alignmentAxis);
  const side = getSide$1(placement);
  const isVertical = sideAxis === "y";
  const commonX = reference.x + reference.width / 2 - floating.width / 2;
  const commonY = reference.y + reference.height / 2 - floating.height / 2;
  const commonAlign = reference[alignLength] / 2 - floating[alignLength] / 2;
  let coords;
  switch (side) {
    case "top":
      coords = {
        x: commonX,
        y: reference.y - floating.height
      };
      break;
    case "bottom":
      coords = {
        x: commonX,
        y: reference.y + reference.height
      };
      break;
    case "right":
      coords = {
        x: reference.x + reference.width,
        y: commonY
      };
      break;
    case "left":
      coords = {
        x: reference.x - floating.width,
        y: commonY
      };
      break;
    default:
      coords = {
        x: reference.x,
        y: reference.y
      };
  }
  const alignment = getAlignment(placement);
  if (alignment) {
    coords[alignmentAxis] += commonAlign * (alignment === "end" ? 1 : -1) * (rtl && isVertical ? -1 : 1);
  }
  return coords;
}
async function detectOverflow(state2, options) {
  var _await$platform$isEle;
  if (options === void 0) {
    options = {};
  }
  const {
    x,
    y,
    platform: platform2,
    rects,
    elements,
    strategy
  } = state2;
  const {
    boundary = "clippingAncestors",
    rootBoundary = "viewport",
    elementContext = "floating",
    altBoundary = false,
    padding = 0
  } = evaluate(options, state2);
  const paddingObject = getPaddingObject(padding);
  const altContext = elementContext === "floating" ? "reference" : "floating";
  const element2 = elements[altBoundary ? altContext : elementContext];
  const clippingClientRect = rectToClientRect(await platform2.getClippingRect({
    element: ((_await$platform$isEle = await (platform2.isElement == null ? void 0 : platform2.isElement(element2))) != null ? _await$platform$isEle : true) ? element2 : element2.contextElement || await (platform2.getDocumentElement == null ? void 0 : platform2.getDocumentElement(elements.floating)),
    boundary,
    rootBoundary,
    strategy
  }));
  const rect = elementContext === "floating" ? {
    x,
    y,
    width: rects.floating.width,
    height: rects.floating.height
  } : rects.reference;
  const offsetParent = await (platform2.getOffsetParent == null ? void 0 : platform2.getOffsetParent(elements.floating));
  const offsetScale = await (platform2.isElement == null ? void 0 : platform2.isElement(offsetParent)) && await (platform2.getScale == null ? void 0 : platform2.getScale(offsetParent)) || {
    x: 1,
    y: 1
  };
  const elementClientRect = rectToClientRect(platform2.convertOffsetParentRelativeRectToViewportRelativeRect ? await platform2.convertOffsetParentRelativeRectToViewportRelativeRect({
    elements,
    rect,
    offsetParent,
    strategy
  }) : rect);
  return {
    top: (clippingClientRect.top - elementClientRect.top + paddingObject.top) / offsetScale.y,
    bottom: (elementClientRect.bottom - clippingClientRect.bottom + paddingObject.bottom) / offsetScale.y,
    left: (clippingClientRect.left - elementClientRect.left + paddingObject.left) / offsetScale.x,
    right: (elementClientRect.right - clippingClientRect.right + paddingObject.right) / offsetScale.x
  };
}
const MAX_RESET_COUNT = 50;
const computePosition$1 = async (reference, floating, config) => {
  const {
    placement = "bottom",
    strategy = "absolute",
    middleware = [],
    platform: platform2
  } = config;
  const platformWithDetectOverflow = platform2.detectOverflow ? platform2 : {
    ...platform2,
    detectOverflow
  };
  const rtl = await (platform2.isRTL == null ? void 0 : platform2.isRTL(floating));
  let rects = await platform2.getElementRects({
    reference,
    floating,
    strategy
  });
  let {
    x,
    y
  } = computeCoordsFromPlacement(rects, placement, rtl);
  let statefulPlacement = placement;
  let resetCount = 0;
  const middlewareData = {};
  for (let i = 0; i < middleware.length; i++) {
    const currentMiddleware = middleware[i];
    if (!currentMiddleware) {
      continue;
    }
    const {
      name,
      fn
    } = currentMiddleware;
    const {
      x: nextX,
      y: nextY,
      data,
      reset
    } = await fn({
      x,
      y,
      initialPlacement: placement,
      placement: statefulPlacement,
      strategy,
      middlewareData,
      rects,
      platform: platformWithDetectOverflow,
      elements: {
        reference,
        floating
      }
    });
    x = nextX != null ? nextX : x;
    y = nextY != null ? nextY : y;
    middlewareData[name] = {
      ...middlewareData[name],
      ...data
    };
    if (reset && resetCount < MAX_RESET_COUNT) {
      resetCount++;
      if (typeof reset === "object") {
        if (reset.placement) {
          statefulPlacement = reset.placement;
        }
        if (reset.rects) {
          rects = reset.rects === true ? await platform2.getElementRects({
            reference,
            floating,
            strategy
          }) : reset.rects;
        }
        ({
          x,
          y
        } = computeCoordsFromPlacement(rects, statefulPlacement, rtl));
      }
      i = -1;
    }
  }
  return {
    x,
    y,
    placement: statefulPlacement,
    strategy,
    middlewareData
  };
};
const arrow$1 = (options) => ({
  name: "arrow",
  options,
  async fn(state2) {
    const {
      x,
      y,
      placement,
      rects,
      platform: platform2,
      elements,
      middlewareData
    } = state2;
    const {
      element: element2,
      padding = 0
    } = evaluate(options, state2) || {};
    if (element2 == null) {
      return {};
    }
    const paddingObject = getPaddingObject(padding);
    const coords = {
      x,
      y
    };
    const axis = getAlignmentAxis(placement);
    const length = getAxisLength(axis);
    const arrowDimensions = await platform2.getDimensions(element2);
    const isYAxis = axis === "y";
    const minProp = isYAxis ? "top" : "left";
    const maxProp = isYAxis ? "bottom" : "right";
    const clientProp = isYAxis ? "clientHeight" : "clientWidth";
    const endDiff = rects.reference[length] + rects.reference[axis] - coords[axis] - rects.floating[length];
    const startDiff = coords[axis] - rects.reference[axis];
    const arrowOffsetParent = await (platform2.getOffsetParent == null ? void 0 : platform2.getOffsetParent(element2));
    let clientSize = arrowOffsetParent ? arrowOffsetParent[clientProp] : 0;
    if (!clientSize || !await (platform2.isElement == null ? void 0 : platform2.isElement(arrowOffsetParent))) {
      clientSize = elements.floating[clientProp] || rects.floating[length];
    }
    const centerToReference = endDiff / 2 - startDiff / 2;
    const largestPossiblePadding = clientSize / 2 - arrowDimensions[length] / 2 - 1;
    const minPadding = min(paddingObject[minProp], largestPossiblePadding);
    const maxPadding = min(paddingObject[maxProp], largestPossiblePadding);
    const max2 = clientSize - arrowDimensions[length] - maxPadding;
    const center = clientSize / 2 - arrowDimensions[length] / 2 + centerToReference;
    const offset2 = clamp(minPadding, center, max2);
    const shouldAddOffset = !middlewareData.arrow && getAlignment(placement) != null && center !== offset2 && rects.reference[length] / 2 - (center < minPadding ? minPadding : maxPadding) - arrowDimensions[length] / 2 < 0;
    const alignmentOffset = shouldAddOffset ? center < minPadding ? center - minPadding : center - max2 : 0;
    return {
      [axis]: coords[axis] + alignmentOffset,
      data: {
        [axis]: offset2,
        centerOffset: center - offset2 - alignmentOffset,
        ...shouldAddOffset && {
          alignmentOffset
        }
      },
      reset: shouldAddOffset
    };
  }
});
const flip$1 = function(options) {
  if (options === void 0) {
    options = {};
  }
  return {
    name: "flip",
    options,
    async fn(state2) {
      var _middlewareData$arrow, _middlewareData$flip;
      const {
        placement,
        middlewareData,
        rects,
        initialPlacement,
        platform: platform2,
        elements
      } = state2;
      const {
        mainAxis: checkMainAxis = true,
        crossAxis: checkCrossAxis = true,
        fallbackPlacements: specifiedFallbackPlacements,
        fallbackStrategy = "bestFit",
        fallbackAxisSideDirection = "none",
        flipAlignment = true,
        ...detectOverflowOptions
      } = evaluate(options, state2);
      if ((_middlewareData$arrow = middlewareData.arrow) != null && _middlewareData$arrow.alignmentOffset) {
        return {};
      }
      const side = getSide$1(placement);
      const initialSideAxis = getSideAxis(initialPlacement);
      const isBasePlacement = getSide$1(initialPlacement) === initialPlacement;
      const rtl = await (platform2.isRTL == null ? void 0 : platform2.isRTL(elements.floating));
      const fallbackPlacements = specifiedFallbackPlacements || (isBasePlacement || !flipAlignment ? [getOppositePlacement(initialPlacement)] : getExpandedPlacements(initialPlacement));
      const hasFallbackAxisSideDirection = fallbackAxisSideDirection !== "none";
      if (!specifiedFallbackPlacements && hasFallbackAxisSideDirection) {
        fallbackPlacements.push(...getOppositeAxisPlacements(initialPlacement, flipAlignment, fallbackAxisSideDirection, rtl));
      }
      const placements = [initialPlacement, ...fallbackPlacements];
      const overflow = await platform2.detectOverflow(state2, detectOverflowOptions);
      const overflows = [];
      let overflowsData = ((_middlewareData$flip = middlewareData.flip) == null ? void 0 : _middlewareData$flip.overflows) || [];
      if (checkMainAxis) {
        overflows.push(overflow[side]);
      }
      if (checkCrossAxis) {
        const sides2 = getAlignmentSides(placement, rects, rtl);
        overflows.push(overflow[sides2[0]], overflow[sides2[1]]);
      }
      overflowsData = [...overflowsData, {
        placement,
        overflows
      }];
      if (!overflows.every((side2) => side2 <= 0)) {
        var _middlewareData$flip2, _overflowsData$filter;
        const nextIndex = (((_middlewareData$flip2 = middlewareData.flip) == null ? void 0 : _middlewareData$flip2.index) || 0) + 1;
        const nextPlacement = placements[nextIndex];
        if (nextPlacement) {
          const ignoreCrossAxisOverflow = checkCrossAxis === "alignment" ? initialSideAxis !== getSideAxis(nextPlacement) : false;
          if (!ignoreCrossAxisOverflow || // We leave the current main axis only if every placement on that axis
          // overflows the main axis.
          overflowsData.every((d) => getSideAxis(d.placement) === initialSideAxis ? d.overflows[0] > 0 : true)) {
            return {
              data: {
                index: nextIndex,
                overflows: overflowsData
              },
              reset: {
                placement: nextPlacement
              }
            };
          }
        }
        let resetPlacement = (_overflowsData$filter = overflowsData.filter((d) => d.overflows[0] <= 0).sort((a, b) => a.overflows[1] - b.overflows[1])[0]) == null ? void 0 : _overflowsData$filter.placement;
        if (!resetPlacement) {
          switch (fallbackStrategy) {
            case "bestFit": {
              var _overflowsData$filter2;
              const placement2 = (_overflowsData$filter2 = overflowsData.filter((d) => {
                if (hasFallbackAxisSideDirection) {
                  const currentSideAxis = getSideAxis(d.placement);
                  return currentSideAxis === initialSideAxis || // Create a bias to the `y` side axis due to horizontal
                  // reading directions favoring greater width.
                  currentSideAxis === "y";
                }
                return true;
              }).map((d) => [d.placement, d.overflows.filter((overflow2) => overflow2 > 0).reduce((acc, overflow2) => acc + overflow2, 0)]).sort((a, b) => a[1] - b[1])[0]) == null ? void 0 : _overflowsData$filter2[0];
              if (placement2) {
                resetPlacement = placement2;
              }
              break;
            }
            case "initialPlacement":
              resetPlacement = initialPlacement;
              break;
          }
        }
        if (placement !== resetPlacement) {
          return {
            reset: {
              placement: resetPlacement
            }
          };
        }
      }
      return {};
    }
  };
};
function getSideOffsets(overflow, rect) {
  return {
    top: overflow.top - rect.height,
    right: overflow.right - rect.width,
    bottom: overflow.bottom - rect.height,
    left: overflow.left - rect.width
  };
}
function isAnySideFullyClipped(overflow) {
  return sides.some((side) => overflow[side] >= 0);
}
const hide$1 = function(options) {
  if (options === void 0) {
    options = {};
  }
  return {
    name: "hide",
    options,
    async fn(state2) {
      const {
        rects,
        platform: platform2
      } = state2;
      const {
        strategy = "referenceHidden",
        ...detectOverflowOptions
      } = evaluate(options, state2);
      switch (strategy) {
        case "referenceHidden": {
          const overflow = await platform2.detectOverflow(state2, {
            ...detectOverflowOptions,
            elementContext: "reference"
          });
          const offsets = getSideOffsets(overflow, rects.reference);
          return {
            data: {
              referenceHiddenOffsets: offsets,
              referenceHidden: isAnySideFullyClipped(offsets)
            }
          };
        }
        case "escaped": {
          const overflow = await platform2.detectOverflow(state2, {
            ...detectOverflowOptions,
            altBoundary: true
          });
          const offsets = getSideOffsets(overflow, rects.floating);
          return {
            data: {
              escapedOffsets: offsets,
              escaped: isAnySideFullyClipped(offsets)
            }
          };
        }
        default: {
          return {};
        }
      }
    }
  };
};
const originSides = /* @__PURE__ */ new Set(["left", "top"]);
async function convertValueToCoords(state2, options) {
  const {
    placement,
    platform: platform2,
    elements
  } = state2;
  const rtl = await (platform2.isRTL == null ? void 0 : platform2.isRTL(elements.floating));
  const side = getSide$1(placement);
  const alignment = getAlignment(placement);
  const isVertical = getSideAxis(placement) === "y";
  const mainAxisMulti = originSides.has(side) ? -1 : 1;
  const crossAxisMulti = rtl && isVertical ? -1 : 1;
  const rawValue = evaluate(options, state2);
  let {
    mainAxis,
    crossAxis,
    alignmentAxis
  } = typeof rawValue === "number" ? {
    mainAxis: rawValue,
    crossAxis: 0,
    alignmentAxis: null
  } : {
    mainAxis: rawValue.mainAxis || 0,
    crossAxis: rawValue.crossAxis || 0,
    alignmentAxis: rawValue.alignmentAxis
  };
  if (alignment && typeof alignmentAxis === "number") {
    crossAxis = alignment === "end" ? alignmentAxis * -1 : alignmentAxis;
  }
  return isVertical ? {
    x: crossAxis * crossAxisMulti,
    y: mainAxis * mainAxisMulti
  } : {
    x: mainAxis * mainAxisMulti,
    y: crossAxis * crossAxisMulti
  };
}
const offset$1 = function(options) {
  if (options === void 0) {
    options = 0;
  }
  return {
    name: "offset",
    options,
    async fn(state2) {
      var _middlewareData$offse, _middlewareData$arrow;
      const {
        x,
        y,
        placement,
        middlewareData
      } = state2;
      const diffCoords = await convertValueToCoords(state2, options);
      if (placement === ((_middlewareData$offse = middlewareData.offset) == null ? void 0 : _middlewareData$offse.placement) && (_middlewareData$arrow = middlewareData.arrow) != null && _middlewareData$arrow.alignmentOffset) {
        return {};
      }
      return {
        x: x + diffCoords.x,
        y: y + diffCoords.y,
        data: {
          ...diffCoords,
          placement
        }
      };
    }
  };
};
const shift$1 = function(options) {
  if (options === void 0) {
    options = {};
  }
  return {
    name: "shift",
    options,
    async fn(state2) {
      const {
        x,
        y,
        placement,
        platform: platform2
      } = state2;
      const {
        mainAxis: checkMainAxis = true,
        crossAxis: checkCrossAxis = false,
        limiter = {
          fn: (_ref) => {
            let {
              x: x2,
              y: y2
            } = _ref;
            return {
              x: x2,
              y: y2
            };
          }
        },
        ...detectOverflowOptions
      } = evaluate(options, state2);
      const coords = {
        x,
        y
      };
      const overflow = await platform2.detectOverflow(state2, detectOverflowOptions);
      const crossAxis = getSideAxis(placement);
      const mainAxis = getOppositeAxis(crossAxis);
      let mainAxisCoord = coords[mainAxis];
      let crossAxisCoord = coords[crossAxis];
      const clampCoord = (axis, coord) => clamp(coord + overflow[axis === "y" ? "top" : "left"], coord, coord - overflow[axis === "y" ? "bottom" : "right"]);
      if (checkMainAxis) {
        mainAxisCoord = clampCoord(mainAxis, mainAxisCoord);
      }
      if (checkCrossAxis) {
        crossAxisCoord = clampCoord(crossAxis, crossAxisCoord);
      }
      const limitedCoords = limiter.fn({
        ...state2,
        [mainAxis]: mainAxisCoord,
        [crossAxis]: crossAxisCoord
      });
      return {
        ...limitedCoords,
        data: {
          x: limitedCoords.x - x,
          y: limitedCoords.y - y,
          enabled: {
            [mainAxis]: checkMainAxis,
            [crossAxis]: checkCrossAxis
          }
        }
      };
    }
  };
};
const limitShift$1 = function(options) {
  if (options === void 0) {
    options = {};
  }
  return {
    options,
    fn(state2) {
      var _rawOffset$mainAxis, _rawOffset$crossAxis;
      const {
        x,
        y,
        placement,
        rects,
        middlewareData
      } = state2;
      const {
        offset: offset2 = 0,
        mainAxis: checkMainAxis = true,
        crossAxis: checkCrossAxis = true
      } = evaluate(options, state2);
      const coords = {
        x,
        y
      };
      const crossAxis = getSideAxis(placement);
      const mainAxis = getOppositeAxis(crossAxis);
      let mainAxisCoord = coords[mainAxis];
      let crossAxisCoord = coords[crossAxis];
      const rawOffset = evaluate(offset2, state2);
      const computedOffset = typeof rawOffset === "number" ? {
        mainAxis: rawOffset,
        crossAxis: 0
      } : {
        mainAxis: (_rawOffset$mainAxis = rawOffset.mainAxis) != null ? _rawOffset$mainAxis : 0,
        crossAxis: (_rawOffset$crossAxis = rawOffset.crossAxis) != null ? _rawOffset$crossAxis : 0
      };
      if (checkMainAxis) {
        const len = mainAxis === "y" ? "height" : "width";
        const limitMin = rects.reference[mainAxis] - rects.floating[len] + computedOffset.mainAxis;
        const limitMax = rects.reference[mainAxis] + rects.reference[len] - computedOffset.mainAxis;
        if (mainAxisCoord < limitMin) {
          mainAxisCoord = limitMin;
        } else if (mainAxisCoord > limitMax) {
          mainAxisCoord = limitMax;
        }
      }
      if (checkCrossAxis) {
        var _middlewareData$offse, _middlewareData$offse2;
        const len = mainAxis === "y" ? "width" : "height";
        const isOriginSide = originSides.has(getSide$1(placement));
        const limitMin = rects.reference[crossAxis] - rects.floating[len] + (isOriginSide ? ((_middlewareData$offse = middlewareData.offset) == null ? void 0 : _middlewareData$offse[crossAxis]) || 0 : 0) + (isOriginSide ? 0 : computedOffset.crossAxis);
        const limitMax = rects.reference[crossAxis] + rects.reference[len] + (isOriginSide ? 0 : ((_middlewareData$offse2 = middlewareData.offset) == null ? void 0 : _middlewareData$offse2[crossAxis]) || 0) - (isOriginSide ? computedOffset.crossAxis : 0);
        if (crossAxisCoord < limitMin) {
          crossAxisCoord = limitMin;
        } else if (crossAxisCoord > limitMax) {
          crossAxisCoord = limitMax;
        }
      }
      return {
        [mainAxis]: mainAxisCoord,
        [crossAxis]: crossAxisCoord
      };
    }
  };
};
const size$1 = function(options) {
  if (options === void 0) {
    options = {};
  }
  return {
    name: "size",
    options,
    async fn(state2) {
      const {
        placement,
        rects,
        platform: platform2,
        elements
      } = state2;
      const {
        apply = () => {
        },
        ...detectOverflowOptions
      } = evaluate(options, state2);
      const overflow = await platform2.detectOverflow(state2, detectOverflowOptions);
      const side = getSide$1(placement);
      const alignment = getAlignment(placement);
      const isYAxis = getSideAxis(placement) === "y";
      const {
        width,
        height
      } = rects.floating;
      let heightSide;
      let widthSide;
      if (side === "top" || side === "bottom") {
        heightSide = side;
        widthSide = alignment === (await (platform2.isRTL == null ? void 0 : platform2.isRTL(elements.floating)) ? "start" : "end") ? "left" : "right";
      } else {
        widthSide = side;
        heightSide = alignment === "end" ? "top" : "bottom";
      }
      const maximumClippingHeight = height - overflow.top - overflow.bottom;
      const maximumClippingWidth = width - overflow.left - overflow.right;
      const overflowAvailableHeight = min(height - overflow[heightSide], maximumClippingHeight);
      const overflowAvailableWidth = min(width - overflow[widthSide], maximumClippingWidth);
      const shiftData = state2.middlewareData.shift;
      const noShift = !shiftData;
      let availableHeight = overflowAvailableHeight;
      let availableWidth = overflowAvailableWidth;
      if (shiftData != null && shiftData.enabled.x) {
        availableWidth = maximumClippingWidth;
      }
      if (shiftData != null && shiftData.enabled.y) {
        availableHeight = maximumClippingHeight;
      }
      if (noShift && !alignment) {
        if (isYAxis) {
          availableWidth = width - 2 * max(overflow.left, overflow.right);
        } else {
          availableHeight = height - 2 * max(overflow.top, overflow.bottom);
        }
      }
      await apply({
        ...state2,
        availableWidth,
        availableHeight
      });
      const nextDimensions = await platform2.getDimensions(elements.floating);
      if (width !== nextDimensions.width || height !== nextDimensions.height) {
        return {
          reset: {
            rects: true
          }
        };
      }
      return {};
    }
  };
};
function hasWindow() {
  return typeof window !== "undefined";
}
function getNodeName(node) {
  if (isNode(node)) {
    return (node.nodeName || "").toLowerCase();
  }
  return "#document";
}
function getWindow(node) {
  var _node$ownerDocument;
  return (node == null || (_node$ownerDocument = node.ownerDocument) == null ? void 0 : _node$ownerDocument.defaultView) || window;
}
function getDocumentElement(node) {
  var _ref;
  return (_ref = (isNode(node) ? node.ownerDocument : node.document) || window.document) == null ? void 0 : _ref.documentElement;
}
function isNode(value) {
  if (!hasWindow()) {
    return false;
  }
  return value instanceof Node || value instanceof getWindow(value).Node;
}
function isElement(value) {
  if (!hasWindow()) {
    return false;
  }
  return value instanceof Element || value instanceof getWindow(value).Element;
}
function isHTMLElement(value) {
  if (!hasWindow()) {
    return false;
  }
  return value instanceof HTMLElement || value instanceof getWindow(value).HTMLElement;
}
function isShadowRoot(value) {
  if (!hasWindow() || typeof ShadowRoot === "undefined") {
    return false;
  }
  return value instanceof ShadowRoot || value instanceof getWindow(value).ShadowRoot;
}
function isOverflowElement(element2) {
  const {
    overflow,
    overflowX,
    overflowY,
    display
  } = getComputedStyle$1(element2);
  return /auto|scroll|overlay|hidden|clip/.test(overflow + overflowY + overflowX) && display !== "inline" && display !== "contents";
}
function isTableElement(element2) {
  return /^(table|td|th)$/.test(getNodeName(element2));
}
function isTopLayer(element2) {
  try {
    if (element2.matches(":popover-open")) {
      return true;
    }
  } catch (_e) {
  }
  try {
    return element2.matches(":modal");
  } catch (_e) {
    return false;
  }
}
const willChangeRe = /transform|translate|scale|rotate|perspective|filter/;
const containRe = /paint|layout|strict|content/;
const isNotNone = (value) => !!value && value !== "none";
let isWebKitValue;
function isContainingBlock(elementOrCss) {
  const css = isElement(elementOrCss) ? getComputedStyle$1(elementOrCss) : elementOrCss;
  return isNotNone(css.transform) || isNotNone(css.translate) || isNotNone(css.scale) || isNotNone(css.rotate) || isNotNone(css.perspective) || !isWebKit() && (isNotNone(css.backdropFilter) || isNotNone(css.filter)) || willChangeRe.test(css.willChange || "") || containRe.test(css.contain || "");
}
function getContainingBlock(element2) {
  let currentNode = getParentNode(element2);
  while (isHTMLElement(currentNode) && !isLastTraversableNode(currentNode)) {
    if (isContainingBlock(currentNode)) {
      return currentNode;
    } else if (isTopLayer(currentNode)) {
      return null;
    }
    currentNode = getParentNode(currentNode);
  }
  return null;
}
function isWebKit() {
  if (isWebKitValue == null) {
    isWebKitValue = typeof CSS !== "undefined" && CSS.supports && CSS.supports("-webkit-backdrop-filter", "none");
  }
  return isWebKitValue;
}
function isLastTraversableNode(node) {
  return /^(html|body|#document)$/.test(getNodeName(node));
}
function getComputedStyle$1(element2) {
  return getWindow(element2).getComputedStyle(element2);
}
function getNodeScroll(element2) {
  if (isElement(element2)) {
    return {
      scrollLeft: element2.scrollLeft,
      scrollTop: element2.scrollTop
    };
  }
  return {
    scrollLeft: element2.scrollX,
    scrollTop: element2.scrollY
  };
}
function getParentNode(node) {
  if (getNodeName(node) === "html") {
    return node;
  }
  const result = (
    // Step into the shadow DOM of the parent of a slotted node.
    node.assignedSlot || // DOM Element detected.
    node.parentNode || // ShadowRoot detected.
    isShadowRoot(node) && node.host || // Fallback.
    getDocumentElement(node)
  );
  return isShadowRoot(result) ? result.host : result;
}
function getNearestOverflowAncestor(node) {
  const parentNode = getParentNode(node);
  if (isLastTraversableNode(parentNode)) {
    return (node.ownerDocument || node).body;
  }
  if (isHTMLElement(parentNode) && isOverflowElement(parentNode)) {
    return parentNode;
  }
  return getNearestOverflowAncestor(parentNode);
}
function getOverflowAncestors(node, list, traverseIframes) {
  var _node$ownerDocument2;
  if (list === void 0) {
    list = [];
  }
  const scrollableAncestor = getNearestOverflowAncestor(node);
  const isBody = scrollableAncestor === ((_node$ownerDocument2 = node.ownerDocument) == null ? void 0 : _node$ownerDocument2.body);
  const win = getWindow(scrollableAncestor);
  if (isBody) {
    getFrameElement(win);
    return list.concat(win, win.visualViewport || [], isOverflowElement(scrollableAncestor) ? scrollableAncestor : [], []);
  } else {
    return list.concat(scrollableAncestor, getOverflowAncestors(scrollableAncestor, []));
  }
}
function getFrameElement(win) {
  return win.parent && Object.getPrototypeOf(win.parent) ? win.frameElement : null;
}
function getCssDimensions(element2) {
  const css = getComputedStyle$1(element2);
  let width = parseFloat(css.width) || 0;
  let height = parseFloat(css.height) || 0;
  const hasOffset = isHTMLElement(element2);
  const offsetWidth = hasOffset ? element2.offsetWidth : width;
  const offsetHeight = hasOffset ? element2.offsetHeight : height;
  const shouldFallback = round(width) !== offsetWidth || round(height) !== offsetHeight;
  if (shouldFallback) {
    width = offsetWidth;
    height = offsetHeight;
  }
  return {
    width,
    height,
    $: shouldFallback
  };
}
function unwrapElement(element2) {
  return !isElement(element2) ? element2.contextElement : element2;
}
function getScale(element2) {
  const domElement = unwrapElement(element2);
  if (!isHTMLElement(domElement)) {
    return createCoords(1);
  }
  const rect = domElement.getBoundingClientRect();
  const {
    width,
    height,
    $
  } = getCssDimensions(domElement);
  let x = ($ ? round(rect.width) : rect.width) / width;
  let y = ($ ? round(rect.height) : rect.height) / height;
  if (!x || !Number.isFinite(x)) {
    x = 1;
  }
  if (!y || !Number.isFinite(y)) {
    y = 1;
  }
  return {
    x,
    y
  };
}
const noOffsets = /* @__PURE__ */ createCoords(0);
function getVisualOffsets(element2) {
  const win = getWindow(element2);
  if (!isWebKit() || !win.visualViewport) {
    return noOffsets;
  }
  return {
    x: win.visualViewport.offsetLeft,
    y: win.visualViewport.offsetTop
  };
}
function shouldAddVisualOffsets(element2, isFixed, floatingOffsetParent) {
  if (isFixed === void 0) {
    isFixed = false;
  }
  return !!floatingOffsetParent && isFixed && floatingOffsetParent === getWindow(element2);
}
function getBoundingClientRect(element2, includeScale, isFixedStrategy, offsetParent) {
  if (includeScale === void 0) {
    includeScale = false;
  }
  if (isFixedStrategy === void 0) {
    isFixedStrategy = false;
  }
  const clientRect = element2.getBoundingClientRect();
  const domElement = unwrapElement(element2);
  let scale = createCoords(1);
  if (includeScale) {
    if (offsetParent) {
      if (isElement(offsetParent)) {
        scale = getScale(offsetParent);
      }
    } else {
      scale = getScale(element2);
    }
  }
  const visualOffsets = shouldAddVisualOffsets(domElement, isFixedStrategy, offsetParent) ? getVisualOffsets(domElement) : createCoords(0);
  let x = (clientRect.left + visualOffsets.x) / scale.x;
  let y = (clientRect.top + visualOffsets.y) / scale.y;
  let width = clientRect.width / scale.x;
  let height = clientRect.height / scale.y;
  if (domElement && offsetParent) {
    const win = getWindow(domElement);
    const offsetWin = isElement(offsetParent) ? getWindow(offsetParent) : offsetParent;
    let currentWin = win;
    let currentIFrame = getFrameElement(currentWin);
    while (currentIFrame && offsetWin !== currentWin) {
      const iframeScale = getScale(currentIFrame);
      const iframeRect = currentIFrame.getBoundingClientRect();
      const css = getComputedStyle$1(currentIFrame);
      const left = iframeRect.left + (currentIFrame.clientLeft + parseFloat(css.paddingLeft)) * iframeScale.x;
      const top = iframeRect.top + (currentIFrame.clientTop + parseFloat(css.paddingTop)) * iframeScale.y;
      x *= iframeScale.x;
      y *= iframeScale.y;
      width *= iframeScale.x;
      height *= iframeScale.y;
      x += left;
      y += top;
      currentWin = getWindow(currentIFrame);
      currentIFrame = getFrameElement(currentWin);
    }
  }
  return rectToClientRect({
    width,
    height,
    x,
    y
  });
}
function getWindowScrollBarX(element2, rect) {
  const leftScroll = getNodeScroll(element2).scrollLeft;
  if (!rect) {
    return getBoundingClientRect(getDocumentElement(element2)).left + leftScroll;
  }
  return rect.left + leftScroll;
}
function getHTMLOffset(documentElement, scroll) {
  const htmlRect = documentElement.getBoundingClientRect();
  const x = htmlRect.left + scroll.scrollLeft - getWindowScrollBarX(documentElement, htmlRect);
  const y = htmlRect.top + scroll.scrollTop;
  return {
    x,
    y
  };
}
function convertOffsetParentRelativeRectToViewportRelativeRect(_ref) {
  let {
    elements,
    rect,
    offsetParent,
    strategy
  } = _ref;
  const isFixed = strategy === "fixed";
  const documentElement = getDocumentElement(offsetParent);
  const topLayer = elements ? isTopLayer(elements.floating) : false;
  if (offsetParent === documentElement || topLayer && isFixed) {
    return rect;
  }
  let scroll = {
    scrollLeft: 0,
    scrollTop: 0
  };
  let scale = createCoords(1);
  const offsets = createCoords(0);
  const isOffsetParentAnElement = isHTMLElement(offsetParent);
  if (isOffsetParentAnElement || !isFixed) {
    if (getNodeName(offsetParent) !== "body" || isOverflowElement(documentElement)) {
      scroll = getNodeScroll(offsetParent);
    }
    if (isOffsetParentAnElement) {
      const offsetRect = getBoundingClientRect(offsetParent);
      scale = getScale(offsetParent);
      offsets.x = offsetRect.x + offsetParent.clientLeft;
      offsets.y = offsetRect.y + offsetParent.clientTop;
    }
  }
  const htmlOffset = documentElement && !isOffsetParentAnElement && !isFixed ? getHTMLOffset(documentElement, scroll) : createCoords(0);
  return {
    width: rect.width * scale.x,
    height: rect.height * scale.y,
    x: rect.x * scale.x - scroll.scrollLeft * scale.x + offsets.x + htmlOffset.x,
    y: rect.y * scale.y - scroll.scrollTop * scale.y + offsets.y + htmlOffset.y
  };
}
function getClientRects(element2) {
  return element2.getClientRects ? Array.from(element2.getClientRects()) : [];
}
function getDocumentRect(html) {
  const scroll = getNodeScroll(html);
  const body = html.ownerDocument.body;
  const width = max(html.scrollWidth, html.clientWidth, body.scrollWidth, body.clientWidth);
  const height = max(html.scrollHeight, html.clientHeight, body.scrollHeight, body.clientHeight);
  let x = -scroll.scrollLeft + getWindowScrollBarX(html);
  const y = -scroll.scrollTop;
  if (getComputedStyle$1(body).direction === "rtl") {
    x += max(html.clientWidth, body.clientWidth) - width;
  }
  return {
    width,
    height,
    x,
    y
  };
}
const SCROLLBAR_MAX = 25;
function getViewportRect(element2, strategy, rootBoundary) {
  if (rootBoundary === void 0) {
    rootBoundary = "viewport";
  }
  const isLayoutViewport = rootBoundary === "layoutViewport";
  const win = getWindow(element2);
  const html = getDocumentElement(element2);
  const visualViewport = win.visualViewport;
  let width = html.clientWidth;
  let height = html.clientHeight;
  let x = 0;
  let y = 0;
  if (visualViewport) {
    const layoutRelativeClientCoords = !isWebKit() || strategy === "fixed";
    if (isLayoutViewport) {
      if (!layoutRelativeClientCoords) {
        x = -visualViewport.offsetLeft;
        y = -visualViewport.offsetTop;
      }
    } else {
      width = visualViewport.width;
      height = visualViewport.height;
      if (layoutRelativeClientCoords) {
        x = visualViewport.offsetLeft;
        y = visualViewport.offsetTop;
      }
    }
  }
  const windowScrollbarX = getWindowScrollBarX(html);
  if (windowScrollbarX <= 0) {
    const doc = html.ownerDocument;
    const body = doc.body;
    const bodyStyles = getComputedStyle(body);
    const bodyMarginInline = doc.compatMode === "CSS1Compat" ? parseFloat(bodyStyles.marginLeft) + parseFloat(bodyStyles.marginRight) || 0 : 0;
    const reservedWidth = Math.abs(html.clientWidth - body.clientWidth - bodyMarginInline);
    const gutter = getComputedStyle(html).scrollbarGutter === "stable both-edges" ? reservedWidth / 2 : reservedWidth;
    if (gutter <= SCROLLBAR_MAX) {
      width -= gutter;
    }
  }
  return {
    width,
    height,
    x,
    y
  };
}
function getInnerBoundingClientRect(element2, strategy) {
  const clientRect = getBoundingClientRect(element2, true, strategy === "fixed");
  const top = clientRect.top + element2.clientTop;
  const left = clientRect.left + element2.clientLeft;
  const scale = getScale(element2);
  const width = element2.clientWidth * scale.x;
  const height = element2.clientHeight * scale.y;
  const x = left * scale.x;
  const y = top * scale.y;
  return {
    width,
    height,
    x,
    y
  };
}
function getClientRectFromClippingAncestor(element2, clippingAncestor, strategy) {
  let rect;
  if (clippingAncestor === "viewport" || clippingAncestor === "layoutViewport") {
    rect = getViewportRect(element2, strategy, clippingAncestor);
  } else if (clippingAncestor === "document") {
    rect = getDocumentRect(getDocumentElement(element2));
  } else if (isElement(clippingAncestor)) {
    rect = getInnerBoundingClientRect(clippingAncestor, strategy);
  } else {
    const visualOffsets = getVisualOffsets(element2);
    rect = {
      x: clippingAncestor.x - visualOffsets.x,
      y: clippingAncestor.y - visualOffsets.y,
      width: clippingAncestor.width,
      height: clippingAncestor.height
    };
  }
  return rectToClientRect(rect);
}
function getClippingElementAncestors(element2, cache) {
  const cachedResult = cache.get(element2);
  if (cachedResult) {
    return cachedResult;
  }
  let result = getOverflowAncestors(element2, []).filter((el) => isElement(el) && getNodeName(el) !== "body");
  let lastKeptComputedStyle = null;
  const elementIsFixed = getComputedStyle$1(element2).position === "fixed";
  let currentNode = elementIsFixed ? getParentNode(element2) : element2;
  while (isElement(currentNode) && !isLastTraversableNode(currentNode)) {
    const computedStyle = getComputedStyle$1(currentNode);
    const currentNodeIsContaining = isContainingBlock(currentNode);
    const lastPosition = lastKeptComputedStyle ? lastKeptComputedStyle.position : elementIsFixed ? "fixed" : "";
    const shouldDropCurrentNode = !currentNodeIsContaining && (lastPosition === "fixed" || lastPosition === "absolute" && computedStyle.position === "static");
    if (shouldDropCurrentNode) {
      result = result.filter((ancestor) => ancestor !== currentNode);
    } else {
      lastKeptComputedStyle = computedStyle;
    }
    currentNode = getParentNode(currentNode);
  }
  cache.set(element2, result);
  return result;
}
function getClippingRect(_ref) {
  let {
    element: element2,
    boundary,
    rootBoundary,
    strategy
  } = _ref;
  const elementClippingAncestors = boundary === "clippingAncestors" ? isTopLayer(element2) ? [] : getClippingElementAncestors(element2, this._c) : [].concat(boundary);
  const clippingAncestors = [...elementClippingAncestors, rootBoundary];
  const firstRect = getClientRectFromClippingAncestor(element2, clippingAncestors[0], strategy);
  let top = firstRect.top;
  let right = firstRect.right;
  let bottom = firstRect.bottom;
  let left = firstRect.left;
  for (let i = 1; i < clippingAncestors.length; i++) {
    const rect = getClientRectFromClippingAncestor(element2, clippingAncestors[i], strategy);
    top = max(rect.top, top);
    right = min(rect.right, right);
    bottom = min(rect.bottom, bottom);
    left = max(rect.left, left);
  }
  return {
    width: right - left,
    height: bottom - top,
    x: left,
    y: top
  };
}
function getDimensions(element2) {
  const {
    width,
    height
  } = getCssDimensions(element2);
  return {
    width,
    height
  };
}
function getRectRelativeToOffsetParent(element2, offsetParent, strategy) {
  const isOffsetParentAnElement = isHTMLElement(offsetParent);
  const documentElement = getDocumentElement(offsetParent);
  const isFixed = strategy === "fixed";
  const rect = getBoundingClientRect(element2, true, isFixed, offsetParent);
  let scroll = {
    scrollLeft: 0,
    scrollTop: 0
  };
  const offsets = createCoords(0);
  if (isOffsetParentAnElement || !isFixed) {
    if (getNodeName(offsetParent) !== "body" || isOverflowElement(documentElement)) {
      scroll = getNodeScroll(offsetParent);
    }
    if (isOffsetParentAnElement) {
      const offsetRect = getBoundingClientRect(offsetParent, true, isFixed, offsetParent);
      offsets.x = offsetRect.x + offsetParent.clientLeft;
      offsets.y = offsetRect.y + offsetParent.clientTop;
    }
  }
  if (!isOffsetParentAnElement && documentElement) {
    offsets.x = getWindowScrollBarX(documentElement);
  }
  const htmlOffset = documentElement && !isOffsetParentAnElement && !isFixed ? getHTMLOffset(documentElement, scroll) : createCoords(0);
  const x = rect.left + scroll.scrollLeft - offsets.x - htmlOffset.x;
  const y = rect.top + scroll.scrollTop - offsets.y - htmlOffset.y;
  return {
    x,
    y,
    width: rect.width,
    height: rect.height
  };
}
function isStaticPositioned(element2) {
  return getComputedStyle$1(element2).position === "static";
}
function getTrueOffsetParent(element2, polyfill) {
  if (!isHTMLElement(element2) || getComputedStyle$1(element2).position === "fixed") {
    return null;
  }
  if (polyfill) {
    return polyfill(element2);
  }
  let rawOffsetParent = element2.offsetParent;
  if (getDocumentElement(element2) === rawOffsetParent) {
    rawOffsetParent = rawOffsetParent.ownerDocument.body;
  }
  return rawOffsetParent;
}
function getOffsetParent(element2, polyfill) {
  const win = getWindow(element2);
  if (isTopLayer(element2)) {
    return win;
  }
  if (!isHTMLElement(element2)) {
    let svgOffsetParent = getParentNode(element2);
    while (svgOffsetParent && !isLastTraversableNode(svgOffsetParent)) {
      if (isElement(svgOffsetParent) && !isStaticPositioned(svgOffsetParent)) {
        return svgOffsetParent;
      }
      svgOffsetParent = getParentNode(svgOffsetParent);
    }
    return win;
  }
  let offsetParent = getTrueOffsetParent(element2, polyfill);
  while (offsetParent && isTableElement(offsetParent) && isStaticPositioned(offsetParent)) {
    offsetParent = getTrueOffsetParent(offsetParent, polyfill);
  }
  if (offsetParent && isLastTraversableNode(offsetParent) && isStaticPositioned(offsetParent) && !isContainingBlock(offsetParent)) {
    return win;
  }
  return offsetParent || getContainingBlock(element2) || win;
}
const getElementRects = async function(data) {
  const getOffsetParentFn = this.getOffsetParent || getOffsetParent;
  const getDimensionsFn = this.getDimensions;
  const floatingDimensions = await getDimensionsFn(data.floating);
  return {
    reference: getRectRelativeToOffsetParent(data.reference, await getOffsetParentFn(data.floating), data.strategy),
    floating: {
      x: 0,
      y: 0,
      width: floatingDimensions.width,
      height: floatingDimensions.height
    }
  };
};
function isRTL(element2) {
  return getComputedStyle$1(element2).direction === "rtl";
}
const platform = {
  convertOffsetParentRelativeRectToViewportRelativeRect,
  getDocumentElement,
  getClippingRect,
  getOffsetParent,
  getElementRects,
  getClientRects,
  getDimensions,
  getScale,
  isElement,
  isRTL
};
const offset = offset$1;
const shift = shift$1;
const flip = flip$1;
const size = size$1;
const hide = hide$1;
const arrow = arrow$1;
const limitShift = limitShift$1;
const computePosition = (reference, floating, options) => {
  const cache = /* @__PURE__ */ new Map();
  const mergedOptions = options != null ? options : {};
  const platformWithCache = {
    ...platform,
    ...mergedOptions.platform,
    _c: cache
  };
  return computePosition$1(reference, floating, {
    ...mergedOptions,
    platform: platformWithCache
  });
};
function useFloating(options) {
  const openOption = derived(() => get(options.open) ?? true);
  const middlewareOption = derived(() => get(options.middleware));
  const transformOption = derived(() => get(options.transform) ?? true);
  const placementOption = derived(() => get(options.placement) ?? "bottom");
  const strategyOption = derived(() => get(options.strategy) ?? "absolute");
  const sideOffsetOption = derived(() => get(options.sideOffset) ?? 0);
  const alignOffsetOption = derived(() => get(options.alignOffset) ?? 0);
  const reference = options.reference;
  let x = 0;
  let y = 0;
  const floating = simpleBox(null);
  let strategy = strategyOption();
  let placement = placementOption();
  let middlewareData = {};
  let isPositioned = false;
  let updateRequestId = 0;
  const floatingStyles = derived(() => {
    const xVal = floating.current ? roundByDPR(floating.current, x) : x;
    const yVal = floating.current ? roundByDPR(floating.current, y) : y;
    if (transformOption()) {
      return {
        position: strategy,
        left: "0",
        top: "0",
        transform: `translate(${xVal}px, ${yVal}px)`,
        ...floating.current && getDPR(floating.current) >= 1.5 && { willChange: "transform" }
      };
    }
    return { position: strategy, left: `${xVal}px`, top: `${yVal}px` };
  });
  function update() {
    if (reference.current === null || floating.current === null) return;
    const referenceNode = reference.current;
    const floatingNode = floating.current;
    const requestId = ++updateRequestId;
    computePosition(referenceNode, floatingNode, {
      middleware: middlewareOption(),
      placement: placementOption(),
      strategy: strategyOption()
    }).then((position) => {
      if (requestId !== updateRequestId) return;
      if (reference.current !== referenceNode || floating.current !== floatingNode) return;
      const referenceHidden = isReferenceHidden(referenceNode);
      if (referenceHidden) {
        middlewareData = {
          ...middlewareData,
          hide: {
            // oxlint-disable-next-line no-explicit-any
            ...middlewareData.hide,
            referenceHidden: true
          }
        };
        return;
      }
      if (!openOption() && x !== 0 && y !== 0) {
        const maxExpectedOffset = Math.max(Math.abs(sideOffsetOption()), Math.abs(alignOffsetOption()), 15);
        if (position.x <= maxExpectedOffset && position.y <= maxExpectedOffset) return;
      }
      x = position.x;
      y = position.y;
      strategy = position.strategy;
      placement = position.placement;
      middlewareData = position.middlewareData;
      isPositioned = true;
    });
  }
  return {
    floating,
    reference,
    get strategy() {
      return strategy;
    },
    get placement() {
      return placement;
    },
    get middlewareData() {
      return middlewareData;
    },
    get isPositioned() {
      return isPositioned;
    },
    get floatingStyles() {
      return floatingStyles();
    },
    get update() {
      return update;
    }
  };
}
function isReferenceHidden(node) {
  if (!(node instanceof Element)) return false;
  if (!node.isConnected) return true;
  if (node instanceof HTMLElement && node.hidden) return true;
  return node.getClientRects().length === 0;
}
const OPPOSITE_SIDE = { top: "bottom", right: "left", bottom: "top", left: "right" };
const FloatingContentContext = new Context("Floating.Content");
const _FloatingContentState = class _FloatingContentState {
  constructor(opts, root) {
    __publicField(this, "opts");
    __publicField(this, "root");
    // nodes
    __publicField(this, "contentRef", simpleBox(null));
    __publicField(this, "wrapperRef", simpleBox(null));
    __publicField(this, "arrowRef", simpleBox(null));
    __publicField(this, "contentAttachment", attachRef(this.contentRef));
    __publicField(this, "wrapperAttachment", attachRef(this.wrapperRef));
    __publicField(this, "arrowAttachment", attachRef(this.arrowRef));
    // ids
    __publicField(this, "arrowId", simpleBox(useId()));
    __privateAdd(this, _transformedStyle, derived(() => {
      if (typeof this.opts.style === "string") return cssToStyleObj(this.opts.style);
      if (!this.opts.style) return {};
    }));
    __privateAdd(this, _updatePositionStrategy);
    __privateAdd(this, _arrowSize, new ElementSize(() => this.arrowRef.current ?? void 0));
    __privateAdd(this, _arrowWidth, derived(() => {
      var _a;
      return ((_a = __privateGet(this, _arrowSize)) == null ? void 0 : _a.width) ?? 0;
    }));
    __privateAdd(this, _arrowHeight, derived(() => {
      var _a;
      return ((_a = __privateGet(this, _arrowSize)) == null ? void 0 : _a.height) ?? 0;
    }));
    __privateAdd(this, _desiredPlacement, derived(() => {
      var _a;
      return ((_a = this.opts.side) == null ? void 0 : _a.current) + (this.opts.align.current !== "center" ? `-${this.opts.align.current}` : "");
    }));
    __privateAdd(this, _boundary2, derived(() => Array.isArray(this.opts.collisionBoundary.current) ? this.opts.collisionBoundary.current : [this.opts.collisionBoundary.current]));
    __privateAdd(this, _hasExplicitBoundaries, derived(() => __privateGet(this, _boundary2).call(this).length > 0));
    __privateAdd(this, _detectOverflowOptions, derived(() => ({
      padding: this.opts.collisionPadding.current,
      boundary: __privateGet(this, _boundary2).call(this).filter(isNotNull),
      altBoundary: this.hasExplicitBoundaries
    })));
    __privateAdd(this, _availableWidth);
    __privateAdd(this, _availableHeight);
    __privateAdd(this, _anchorWidth);
    __privateAdd(this, _anchorHeight);
    __privateAdd(this, _middleware, derived(() => [
      offset({
        mainAxis: this.opts.sideOffset.current + __privateGet(this, _arrowHeight).call(this),
        alignmentAxis: this.opts.alignOffset.current
      }),
      this.opts.avoidCollisions.current && shift({
        mainAxis: true,
        crossAxis: false,
        limiter: this.opts.sticky.current === "partial" ? limitShift() : void 0,
        ...this.detectOverflowOptions
      }),
      this.opts.avoidCollisions.current && flip({ ...this.detectOverflowOptions }),
      size({
        ...this.detectOverflowOptions,
        apply: ({ rects, availableWidth, availableHeight }) => {
          const { width: anchorWidth, height: anchorHeight } = rects.reference;
          __privateSet(this, _availableWidth, availableWidth);
          __privateSet(this, _availableHeight, availableHeight);
          __privateSet(this, _anchorWidth, anchorWidth);
          __privateSet(this, _anchorHeight, anchorHeight);
        }
      }),
      this.arrowRef.current && arrow({
        element: this.arrowRef.current,
        padding: this.opts.arrowPadding.current
      }),
      transformOrigin({
        arrowWidth: __privateGet(this, _arrowWidth).call(this),
        arrowHeight: __privateGet(this, _arrowHeight).call(this)
      }),
      this.opts.hideWhenDetached.current && hide({ strategy: "referenceHidden", ...this.detectOverflowOptions })
    ].filter(Boolean)));
    __publicField(this, "floating");
    __privateAdd(this, _placedSide, derived(() => getSideFromPlacement(this.floating.placement)));
    __privateAdd(this, _placedAlign, derived(() => getAlignFromPlacement(this.floating.placement)));
    __privateAdd(this, _arrowX, derived(() => {
      var _a;
      return ((_a = this.floating.middlewareData.arrow) == null ? void 0 : _a.x) ?? 0;
    }));
    __privateAdd(this, _arrowY, derived(() => {
      var _a;
      return ((_a = this.floating.middlewareData.arrow) == null ? void 0 : _a.y) ?? 0;
    }));
    __privateAdd(this, _cannotCenterArrow, derived(() => {
      var _a;
      return ((_a = this.floating.middlewareData.arrow) == null ? void 0 : _a.centerOffset) !== 0;
    }));
    __publicField(this, "contentZIndex");
    __privateAdd(this, _arrowBaseSide, derived(() => OPPOSITE_SIDE[this.placedSide]));
    __privateAdd(this, _wrapperProps, derived(() => {
      var _a, _b, _c;
      return {
        id: this.opts.wrapperId.current,
        "data-bits-floating-content-wrapper": "",
        style: {
          ...this.floating.floatingStyles,
          transform: this.floating.isPositioned ? this.floating.floatingStyles.transform : "translate(0, -200%)",
          minWidth: "max-content",
          zIndex: this.contentZIndex,
          "--bits-floating-transform-origin": `${(_a = this.floating.middlewareData.transformOrigin) == null ? void 0 : _a.x} ${(_b = this.floating.middlewareData.transformOrigin) == null ? void 0 : _b.y}`,
          "--bits-floating-available-width": `${__privateGet(this, _availableWidth)}px`,
          "--bits-floating-available-height": `${__privateGet(this, _availableHeight)}px`,
          "--bits-floating-anchor-width": `${__privateGet(this, _anchorWidth)}px`,
          "--bits-floating-anchor-height": `${__privateGet(this, _anchorHeight)}px`,
          ...((_c = this.floating.middlewareData.hide) == null ? void 0 : _c.referenceHidden) && { visibility: "hidden", "pointer-events": "none" },
          ...__privateGet(this, _transformedStyle).call(this)
        },
        dir: this.opts.dir.current,
        ...this.wrapperAttachment
      };
    }));
    __privateAdd(this, _props, derived(() => ({
      "data-side": this.placedSide,
      "data-align": this.placedAlign,
      style: styleToString({ ...__privateGet(this, _transformedStyle).call(this) }),
      ...this.contentAttachment
    })));
    __privateAdd(this, _arrowStyle, derived(() => ({
      position: "absolute",
      left: this.arrowX ? `${this.arrowX}px` : void 0,
      top: this.arrowY ? `${this.arrowY}px` : void 0,
      [this.arrowBaseSide]: 0,
      "transform-origin": { top: "", right: "0 0", bottom: "center 0", left: "100% 0" }[this.placedSide],
      transform: {
        top: "translateY(100%)",
        right: "translateY(50%) rotate(90deg) translateX(-50%)",
        bottom: "rotate(180deg)",
        left: "translateY(50%) rotate(-90deg) translateX(50%)"
      }[this.placedSide],
      visibility: this.cannotCenterArrow ? "hidden" : void 0
    })));
    this.opts = opts;
    this.root = root;
    __privateSet(this, _updatePositionStrategy, opts.updatePositionStrategy);
    if (opts.customAnchor) {
      this.root.customAnchorNode.current = opts.customAnchor.current;
    }
    watch(() => opts.customAnchor.current, (customAnchor) => {
      this.root.customAnchorNode.current = customAnchor;
    });
    this.floating = useFloating({
      strategy: () => this.opts.strategy.current,
      placement: () => __privateGet(this, _desiredPlacement).call(this),
      middleware: () => this.middleware,
      reference: this.root.anchorNode,
      open: () => this.opts.enabled.current,
      sideOffset: () => this.opts.sideOffset.current,
      alignOffset: () => this.opts.alignOffset.current
    });
    watch(() => this.contentRef.current, (contentNode) => {
      if (!contentNode || !this.opts.enabled.current) return;
      const win = getWindow$1(contentNode);
      const rafId = win.requestAnimationFrame(() => {
        if (this.contentRef.current !== contentNode || !this.opts.enabled.current) return;
        const zIndex = win.getComputedStyle(contentNode).zIndex;
        if (zIndex !== this.contentZIndex) {
          this.contentZIndex = zIndex;
        }
      });
      return () => {
        win.cancelAnimationFrame(rafId);
      };
    });
  }
  static create(opts, tooltip = false) {
    return tooltip ? FloatingContentContext.set(new _FloatingContentState(opts, FloatingTooltipRootContext.get())) : FloatingContentContext.set(new _FloatingContentState(opts, FloatingRootContext.get()));
  }
  get hasExplicitBoundaries() {
    return __privateGet(this, _hasExplicitBoundaries).call(this);
  }
  set hasExplicitBoundaries($$value) {
    return __privateGet(this, _hasExplicitBoundaries).call(this, $$value);
  }
  get detectOverflowOptions() {
    return __privateGet(this, _detectOverflowOptions).call(this);
  }
  set detectOverflowOptions($$value) {
    return __privateGet(this, _detectOverflowOptions).call(this, $$value);
  }
  get middleware() {
    return __privateGet(this, _middleware).call(this);
  }
  set middleware($$value) {
    return __privateGet(this, _middleware).call(this, $$value);
  }
  get placedSide() {
    return __privateGet(this, _placedSide).call(this);
  }
  set placedSide($$value) {
    return __privateGet(this, _placedSide).call(this, $$value);
  }
  get placedAlign() {
    return __privateGet(this, _placedAlign).call(this);
  }
  set placedAlign($$value) {
    return __privateGet(this, _placedAlign).call(this, $$value);
  }
  get arrowX() {
    return __privateGet(this, _arrowX).call(this);
  }
  set arrowX($$value) {
    return __privateGet(this, _arrowX).call(this, $$value);
  }
  get arrowY() {
    return __privateGet(this, _arrowY).call(this);
  }
  set arrowY($$value) {
    return __privateGet(this, _arrowY).call(this, $$value);
  }
  get cannotCenterArrow() {
    return __privateGet(this, _cannotCenterArrow).call(this);
  }
  set cannotCenterArrow($$value) {
    return __privateGet(this, _cannotCenterArrow).call(this, $$value);
  }
  get arrowBaseSide() {
    return __privateGet(this, _arrowBaseSide).call(this);
  }
  set arrowBaseSide($$value) {
    return __privateGet(this, _arrowBaseSide).call(this, $$value);
  }
  get wrapperProps() {
    return __privateGet(this, _wrapperProps).call(this);
  }
  set wrapperProps($$value) {
    return __privateGet(this, _wrapperProps).call(this, $$value);
  }
  get props() {
    return __privateGet(this, _props).call(this);
  }
  set props($$value) {
    return __privateGet(this, _props).call(this, $$value);
  }
  get arrowStyle() {
    return __privateGet(this, _arrowStyle).call(this);
  }
  set arrowStyle($$value) {
    return __privateGet(this, _arrowStyle).call(this, $$value);
  }
};
_transformedStyle = new WeakMap();
_updatePositionStrategy = new WeakMap();
_arrowSize = new WeakMap();
_arrowWidth = new WeakMap();
_arrowHeight = new WeakMap();
_desiredPlacement = new WeakMap();
_boundary2 = new WeakMap();
_hasExplicitBoundaries = new WeakMap();
_detectOverflowOptions = new WeakMap();
_availableWidth = new WeakMap();
_availableHeight = new WeakMap();
_anchorWidth = new WeakMap();
_anchorHeight = new WeakMap();
_middleware = new WeakMap();
_placedSide = new WeakMap();
_placedAlign = new WeakMap();
_arrowX = new WeakMap();
_arrowY = new WeakMap();
_cannotCenterArrow = new WeakMap();
_arrowBaseSide = new WeakMap();
_wrapperProps = new WeakMap();
_props = new WeakMap();
_arrowStyle = new WeakMap();
let FloatingContentState = _FloatingContentState;
function transformOrigin(options) {
  return {
    name: "transformOrigin",
    options,
    fn(data) {
      var _a, _b, _c;
      const { placement, rects, middlewareData } = data;
      const cannotCenterArrow = ((_a = middlewareData.arrow) == null ? void 0 : _a.centerOffset) !== 0;
      const isArrowHidden = cannotCenterArrow;
      const arrowWidth = isArrowHidden ? 0 : options.arrowWidth;
      const arrowHeight = isArrowHidden ? 0 : options.arrowHeight;
      const [placedSide, placedAlign] = getSideAndAlignFromPlacement(placement);
      const noArrowAlign = { start: "0%", center: "50%", end: "100%" }[placedAlign];
      const arrowXCenter = (((_b = middlewareData.arrow) == null ? void 0 : _b.x) ?? 0) + arrowWidth / 2;
      const arrowYCenter = (((_c = middlewareData.arrow) == null ? void 0 : _c.y) ?? 0) + arrowHeight / 2;
      let x = "";
      let y = "";
      if (placedSide === "bottom") {
        x = isArrowHidden ? noArrowAlign : `${arrowXCenter}px`;
        y = `${-arrowHeight}px`;
      } else if (placedSide === "top") {
        x = isArrowHidden ? noArrowAlign : `${arrowXCenter}px`;
        y = `${rects.floating.height + arrowHeight}px`;
      } else if (placedSide === "right") {
        x = `${-arrowHeight}px`;
        y = isArrowHidden ? noArrowAlign : `${arrowYCenter}px`;
      } else if (placedSide === "left") {
        x = `${rects.floating.width + arrowHeight}px`;
        y = isArrowHidden ? noArrowAlign : `${arrowYCenter}px`;
      }
      return { data: { x, y } };
    }
  };
}
function getSideAndAlignFromPlacement(placement) {
  const [side, align = "center"] = placement.split("-");
  return [side, align];
}
function getSideFromPlacement(placement) {
  return getSideAndAlignFromPlacement(placement)[0];
}
function getAlignFromPlacement(placement) {
  return getSideAndAlignFromPlacement(placement)[1];
}
function Floating_layer_content($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      content,
      side = "bottom",
      sideOffset = 0,
      align = "center",
      alignOffset = 0,
      id,
      arrowPadding = 0,
      avoidCollisions = true,
      collisionBoundary = [],
      collisionPadding = 0,
      hideWhenDetached = false,
      onPlaced = () => {
      },
      sticky = "partial",
      updatePositionStrategy = "optimized",
      strategy = "fixed",
      dir = "ltr",
      style = {},
      wrapperId = useId(),
      customAnchor = null,
      enabled,
      tooltip = false
    } = $$props;
    const contentState = FloatingContentState.create(
      {
        side: boxWith(() => side),
        sideOffset: boxWith(() => sideOffset),
        align: boxWith(() => align),
        alignOffset: boxWith(() => alignOffset),
        id: boxWith(() => id),
        arrowPadding: boxWith(() => arrowPadding),
        avoidCollisions: boxWith(() => avoidCollisions),
        collisionBoundary: boxWith(() => collisionBoundary),
        collisionPadding: boxWith(() => collisionPadding),
        hideWhenDetached: boxWith(() => hideWhenDetached),
        onPlaced: boxWith(() => onPlaced),
        sticky: boxWith(() => sticky),
        updatePositionStrategy: boxWith(() => updatePositionStrategy),
        strategy: boxWith(() => strategy),
        dir: boxWith(() => dir),
        style: boxWith(() => style),
        enabled: boxWith(() => enabled),
        wrapperId: boxWith(() => wrapperId),
        customAnchor: boxWith(() => customAnchor)
      },
      tooltip
    );
    const mergedProps = derived(() => mergeProps(contentState.wrapperProps, { style: { pointerEvents: "auto" } }));
    content == null ? void 0 : content($$renderer2, { props: contentState.props, wrapperProps: mergedProps() });
    $$renderer2.push(`<!---->`);
  });
}
function Floating_layer_content_static($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let { content } = $$props;
    content == null ? void 0 : content($$renderer2, { props: {}, wrapperProps: {} });
    $$renderer2.push(`<!---->`);
  });
}
function Popper_content($$renderer, $$props) {
  let {
    content,
    isStatic = false,
    onPlaced,
    $$slots,
    $$events,
    ...restProps
  } = $$props;
  if (isStatic) {
    $$renderer.push("<!--[0-->");
    Floating_layer_content_static($$renderer, { content });
  } else {
    $$renderer.push("<!--[-1-->");
    Floating_layer_content($$renderer, spread_props([{ content, onPlaced }, restProps]));
  }
  $$renderer.push(`<!--]-->`);
}
function Popper_layer_inner($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      popper,
      onEscapeKeydown,
      escapeKeydownBehavior,
      preventOverflowTextSelection,
      id,
      onPointerDown,
      onPointerUp,
      side,
      sideOffset,
      align,
      alignOffset,
      arrowPadding,
      avoidCollisions,
      collisionBoundary,
      collisionPadding,
      sticky,
      hideWhenDetached,
      updatePositionStrategy,
      strategy,
      dir,
      preventScroll,
      wrapperId,
      style,
      onPlaced,
      onInteractOutside,
      onCloseAutoFocus,
      onOpenAutoFocus,
      onFocusOutside,
      interactOutsideBehavior = "close",
      loop,
      trapFocus = true,
      isValidEvent: isValidEvent2 = () => false,
      customAnchor = null,
      isStatic = false,
      enabled,
      forceMount,
      ref,
      tooltip = false,
      contentPointerEvents = "auto",
      $$slots,
      $$events,
      ...restProps
    } = $$props;
    const resolvedPreventScroll = derived(() => preventScroll ?? true);
    const effectiveStrategy = derived(() => strategy ?? (resolvedPreventScroll() ? "fixed" : "absolute"));
    {
      let content = function($$renderer3, { props: floatingProps, wrapperProps }) {
        if (forceMount && enabled) {
          $$renderer3.push("<!--[0-->");
          Scroll_lock($$renderer3, { preventScroll: resolvedPreventScroll() });
        } else if (!forceMount) {
          $$renderer3.push("<!--[1-->");
          Scroll_lock($$renderer3, { preventScroll: resolvedPreventScroll() });
        } else {
          $$renderer3.push("<!--[-1-->");
        }
        $$renderer3.push(`<!--]--> `);
        {
          let focusScope = function($$renderer4, { props: focusScopeProps }) {
            Escape_layer($$renderer4, {
              onEscapeKeydown,
              escapeKeydownBehavior,
              enabled,
              ref,
              children: ($$renderer5) => {
                {
                  let children = function($$renderer6, { props: dismissibleProps }) {
                    Text_selection_layer($$renderer6, {
                      id,
                      preventOverflowTextSelection,
                      onPointerDown,
                      onPointerUp,
                      enabled,
                      ref,
                      children: ($$renderer7) => {
                        popper == null ? void 0 : popper($$renderer7, {
                          props: mergeProps(restProps, floatingProps, dismissibleProps, focusScopeProps, { id, style: { pointerEvents: contentPointerEvents } }),
                          wrapperProps
                        });
                        $$renderer7.push(`<!---->`);
                      }
                    });
                  };
                  Dismissible_layer($$renderer5, {
                    id,
                    onInteractOutside,
                    onFocusOutside,
                    interactOutsideBehavior,
                    isValidEvent: isValidEvent2,
                    enabled,
                    ref,
                    children
                  });
                }
              }
            });
          };
          Focus_scope($$renderer3, {
            onOpenAutoFocus,
            onCloseAutoFocus,
            loop,
            enabled,
            trapFocus,
            ref,
            focusScope
          });
        }
        $$renderer3.push(`<!---->`);
      };
      Popper_content($$renderer2, {
        isStatic,
        id,
        side,
        sideOffset,
        align,
        alignOffset,
        arrowPadding,
        avoidCollisions,
        collisionBoundary,
        collisionPadding,
        sticky,
        hideWhenDetached,
        updatePositionStrategy,
        strategy: effectiveStrategy(),
        dir,
        wrapperId,
        style,
        onPlaced,
        customAnchor,
        enabled,
        tooltip,
        content,
        $$slots: { content: true }
      });
    }
  });
}
function Popper_layer($$renderer, $$props) {
  let {
    popper,
    open,
    onEscapeKeydown,
    escapeKeydownBehavior,
    preventOverflowTextSelection,
    id,
    onPointerDown,
    onPointerUp,
    side,
    sideOffset,
    align,
    alignOffset,
    arrowPadding,
    avoidCollisions,
    collisionBoundary,
    collisionPadding,
    sticky,
    hideWhenDetached,
    updatePositionStrategy,
    strategy,
    dir,
    preventScroll,
    wrapperId,
    style,
    onPlaced,
    onInteractOutside,
    onCloseAutoFocus,
    onOpenAutoFocus,
    onFocusOutside,
    interactOutsideBehavior = "close",
    loop,
    trapFocus = true,
    isValidEvent: isValidEvent2 = () => false,
    customAnchor = null,
    isStatic = false,
    ref,
    shouldRender,
    $$slots,
    $$events,
    ...restProps
  } = $$props;
  if (shouldRender) {
    $$renderer.push("<!--[0-->");
    Popper_layer_inner($$renderer, spread_props([
      {
        popper,
        onEscapeKeydown,
        escapeKeydownBehavior,
        preventOverflowTextSelection,
        id,
        onPointerDown,
        onPointerUp,
        side,
        sideOffset,
        align,
        alignOffset,
        arrowPadding,
        avoidCollisions,
        collisionBoundary,
        collisionPadding,
        sticky,
        hideWhenDetached,
        updatePositionStrategy,
        strategy,
        dir,
        preventScroll,
        wrapperId,
        style,
        onPlaced,
        customAnchor,
        isStatic,
        enabled: open,
        onInteractOutside,
        onCloseAutoFocus,
        onOpenAutoFocus,
        interactOutsideBehavior,
        loop,
        trapFocus,
        isValidEvent: isValidEvent2,
        onFocusOutside,
        forceMount: false,
        ref
      },
      restProps
    ]));
  } else {
    $$renderer.push("<!--[-1-->");
  }
  $$renderer.push(`<!--]-->`);
}
function Popper_layer_force_mount($$renderer, $$props) {
  let {
    popper,
    onEscapeKeydown,
    escapeKeydownBehavior,
    preventOverflowTextSelection,
    id,
    onPointerDown,
    onPointerUp,
    side,
    sideOffset,
    align,
    alignOffset,
    arrowPadding,
    avoidCollisions,
    collisionBoundary,
    collisionPadding,
    sticky,
    hideWhenDetached,
    updatePositionStrategy,
    strategy,
    dir,
    preventScroll,
    wrapperId,
    style,
    onPlaced,
    onInteractOutside,
    onCloseAutoFocus,
    onOpenAutoFocus,
    onFocusOutside,
    interactOutsideBehavior = "close",
    loop,
    trapFocus = true,
    isValidEvent: isValidEvent2 = () => false,
    customAnchor = null,
    isStatic = false,
    enabled,
    $$slots,
    $$events,
    ...restProps
  } = $$props;
  Popper_layer_inner($$renderer, spread_props([
    {
      popper,
      onEscapeKeydown,
      escapeKeydownBehavior,
      preventOverflowTextSelection,
      id,
      onPointerDown,
      onPointerUp,
      side,
      sideOffset,
      align,
      alignOffset,
      arrowPadding,
      avoidCollisions,
      collisionBoundary,
      collisionPadding,
      sticky,
      hideWhenDetached,
      updatePositionStrategy,
      strategy,
      dir,
      preventScroll,
      wrapperId,
      style,
      onPlaced,
      customAnchor,
      isStatic,
      enabled,
      onInteractOutside,
      onCloseAutoFocus,
      onOpenAutoFocus,
      interactOutsideBehavior,
      loop,
      trapFocus,
      isValidEvent: isValidEvent2,
      onFocusOutside
    },
    restProps,
    { forceMount: true }
  ]));
}
function isPointInPolygon(point, polygon) {
  const [x, y] = point;
  let isInside = false;
  const length = polygon.length;
  for (let i = 0, j = length - 1; i < length; j = i++) {
    const [xi, yi] = polygon[i] ?? [0, 0];
    const [xj, yj] = polygon[j] ?? [0, 0];
    const intersect = yi >= y !== yj >= y && x <= (xj - xi) * (y - yi) / (yj - yi) + xi;
    if (intersect) {
      isInside = !isInside;
    }
  }
  return isInside;
}
function isInsideRect(point, rect) {
  return point[0] >= rect.left && point[0] <= rect.right && point[1] >= rect.top && point[1] <= rect.bottom;
}
function getSide(triggerRect, contentRect) {
  const triggerCenterX = triggerRect.left + triggerRect.width / 2;
  const triggerCenterY = triggerRect.top + triggerRect.height / 2;
  const contentCenterX = contentRect.left + contentRect.width / 2;
  const contentCenterY = contentRect.top + contentRect.height / 2;
  const deltaX = contentCenterX - triggerCenterX;
  const deltaY = contentCenterY - triggerCenterY;
  if (Math.abs(deltaX) > Math.abs(deltaY)) {
    return deltaX > 0 ? "right" : "left";
  }
  return deltaY > 0 ? "bottom" : "top";
}
class SafePolygon {
  constructor(opts) {
    __privateAdd(this, _SafePolygon_instances);
    __privateAdd(this, _opts4);
    __privateAdd(this, _buffer);
    __privateAdd(this, _transitIntentTimeout);
    // tracks the cursor position when leaving trigger or content
    __privateAdd(this, _exitPoint, null);
    // tracks what we're moving toward: "content" when leaving trigger, "trigger" when leaving content
    __privateAdd(this, _exitTarget, null);
    __privateAdd(this, _transitTargets, []);
    __privateAdd(this, _trackedTriggerNode, null);
    __privateAdd(this, _leaveFallbackRafId, null);
    __privateAdd(this, _transitIntentTimeoutId, null);
    __privateSet(this, _opts4, opts);
    __privateSet(this, _buffer, opts.buffer ?? 1);
    const transitIntentTimeout = opts.transitIntentTimeout;
    __privateSet(this, _transitIntentTimeout, typeof transitIntentTimeout === "number" && transitIntentTimeout > 0 ? transitIntentTimeout : null);
    watch([opts.triggerNode, opts.contentNode, opts.enabled], ([triggerNode, contentNode, enabled]) => {
      if (!triggerNode || !contentNode || !enabled) {
        __privateSet(this, _trackedTriggerNode, null);
        __privateMethod(this, _SafePolygon_instances, clearTracking_fn).call(this);
        return;
      }
      if (__privateGet(this, _trackedTriggerNode) && __privateGet(this, _trackedTriggerNode) !== triggerNode) {
        __privateMethod(this, _SafePolygon_instances, clearTracking_fn).call(this);
      }
      __privateSet(this, _trackedTriggerNode, triggerNode);
      const doc = getDocument(triggerNode);
      const handlePointerMove = (e) => {
        __privateMethod(this, _SafePolygon_instances, onPointerMove_fn).call(this, [e.clientX, e.clientY], triggerNode, contentNode);
      };
      const handleTriggerLeave = (e) => {
        var _a, _b;
        const target = e.relatedTarget;
        if (isElement$1(target) && contentNode.contains(target)) {
          return;
        }
        const ignoredTargets = ((_b = (_a = __privateGet(this, _opts4)).ignoredTargets) == null ? void 0 : _b.call(_a)) ?? [];
        if (isElement$1(target) && ignoredTargets.some((n) => n === target || n.contains(target))) {
          return;
        }
        __privateSet(this, _transitTargets, isElement$1(target) && ignoredTargets.length > 0 ? ignoredTargets.filter((n) => target.contains(n)) : []);
        __privateSet(this, _exitPoint, [e.clientX, e.clientY]);
        __privateSet(this, _exitTarget, "content");
        __privateMethod(this, _SafePolygon_instances, scheduleLeaveFallback_fn).call(this);
      };
      const handleTriggerEnter = () => {
        __privateMethod(this, _SafePolygon_instances, clearTracking_fn).call(this);
      };
      const handleContentEnter = () => {
        __privateMethod(this, _SafePolygon_instances, clearTracking_fn).call(this);
      };
      const handleContentLeave = (e) => {
        const target = e.relatedTarget;
        if (isElement$1(target) && triggerNode.contains(target)) {
          return;
        }
        __privateSet(this, _exitPoint, [e.clientX, e.clientY]);
        __privateSet(this, _exitTarget, "trigger");
        __privateMethod(this, _SafePolygon_instances, scheduleLeaveFallback_fn).call(this);
      };
      return [
        on(doc, "pointermove", handlePointerMove),
        on(triggerNode, "pointerleave", handleTriggerLeave),
        on(triggerNode, "pointerenter", handleTriggerEnter),
        on(contentNode, "pointerenter", handleContentEnter),
        on(contentNode, "pointerleave", handleContentLeave)
      ].reduce(
        (acc, cleanup) => () => {
          acc();
          cleanup();
        },
        () => {
        }
      );
    });
  }
}
_opts4 = new WeakMap();
_buffer = new WeakMap();
_transitIntentTimeout = new WeakMap();
_exitPoint = new WeakMap();
_exitTarget = new WeakMap();
_transitTargets = new WeakMap();
_trackedTriggerNode = new WeakMap();
_leaveFallbackRafId = new WeakMap();
_transitIntentTimeoutId = new WeakMap();
_SafePolygon_instances = new WeakSet();
cancelLeaveFallback_fn = function() {
  if (__privateGet(this, _leaveFallbackRafId) !== null) {
    cancelAnimationFrame(__privateGet(this, _leaveFallbackRafId));
    __privateSet(this, _leaveFallbackRafId, null);
  }
};
scheduleLeaveFallback_fn = function() {
  __privateMethod(this, _SafePolygon_instances, cancelLeaveFallback_fn).call(this);
  __privateSet(this, _leaveFallbackRafId, requestAnimationFrame(() => {
    __privateSet(this, _leaveFallbackRafId, null);
    if (!__privateGet(this, _exitPoint) || !__privateGet(this, _exitTarget)) return;
    __privateMethod(this, _SafePolygon_instances, clearTracking_fn).call(this);
    __privateGet(this, _opts4).onPointerExit();
  }));
};
cancelTransitIntentTimeout_fn = function() {
  if (__privateGet(this, _transitIntentTimeoutId) !== null) {
    clearTimeout(__privateGet(this, _transitIntentTimeoutId));
    __privateSet(this, _transitIntentTimeoutId, null);
  }
};
scheduleTransitIntentTimeout_fn = function() {
  if (__privateGet(this, _transitIntentTimeout) === null) return;
  __privateMethod(this, _SafePolygon_instances, cancelTransitIntentTimeout_fn).call(this);
  __privateSet(this, _transitIntentTimeoutId, window.setTimeout(
    () => {
      __privateSet(this, _transitIntentTimeoutId, null);
      if (!__privateGet(this, _exitPoint) || !__privateGet(this, _exitTarget)) return;
      __privateMethod(this, _SafePolygon_instances, clearTracking_fn).call(this);
      __privateGet(this, _opts4).onPointerExit();
    },
    __privateGet(this, _transitIntentTimeout)
  ));
};
onPointerMove_fn = function(clientPoint, triggerNode, contentNode) {
  if (!__privateGet(this, _exitPoint) || !__privateGet(this, _exitTarget)) return;
  __privateMethod(this, _SafePolygon_instances, cancelLeaveFallback_fn).call(this);
  __privateMethod(this, _SafePolygon_instances, scheduleTransitIntentTimeout_fn).call(this);
  const triggerRect = triggerNode.getBoundingClientRect();
  const contentRect = contentNode.getBoundingClientRect();
  if (__privateGet(this, _exitTarget) === "content" && isInsideRect(clientPoint, contentRect)) {
    __privateMethod(this, _SafePolygon_instances, clearTracking_fn).call(this);
    return;
  }
  if (__privateGet(this, _exitTarget) === "trigger" && isInsideRect(clientPoint, triggerRect)) {
    __privateMethod(this, _SafePolygon_instances, clearTracking_fn).call(this);
    return;
  }
  if (__privateGet(this, _exitTarget) === "content" && __privateGet(this, _transitTargets).length > 0) {
    for (const transitTarget of __privateGet(this, _transitTargets)) {
      const transitRect = transitTarget.getBoundingClientRect();
      if (isInsideRect(clientPoint, transitRect)) return;
      const transitSide = getSide(triggerRect, transitRect);
      const transitCorridor = __privateMethod(this, _SafePolygon_instances, getCorridorPolygon_fn).call(this, triggerRect, transitRect, transitSide);
      if (transitCorridor && isPointInPolygon(clientPoint, transitCorridor)) return;
    }
  }
  const side = getSide(triggerRect, contentRect);
  const corridorPoly = __privateMethod(this, _SafePolygon_instances, getCorridorPolygon_fn).call(this, triggerRect, contentRect, side);
  if (corridorPoly && isPointInPolygon(clientPoint, corridorPoly)) {
    return;
  }
  const targetRect = __privateGet(this, _exitTarget) === "content" ? contentRect : triggerRect;
  const safePoly = __privateMethod(this, _SafePolygon_instances, getSafePolygon_fn).call(this, __privateGet(this, _exitPoint), targetRect, side, __privateGet(this, _exitTarget));
  if (isPointInPolygon(clientPoint, safePoly)) {
    return;
  }
  __privateMethod(this, _SafePolygon_instances, clearTracking_fn).call(this);
  __privateGet(this, _opts4).onPointerExit();
};
clearTracking_fn = function() {
  __privateSet(this, _exitPoint, null);
  __privateSet(this, _exitTarget, null);
  __privateSet(this, _transitTargets, []);
  __privateMethod(this, _SafePolygon_instances, cancelLeaveFallback_fn).call(this);
  __privateMethod(this, _SafePolygon_instances, cancelTransitIntentTimeout_fn).call(this);
};
/**
 * Creates a rectangular corridor between trigger and content
 * This prevents closing when cursor is in the gap between them
 */
getCorridorPolygon_fn = function(triggerRect, contentRect, side) {
  const buffer = __privateGet(this, _buffer);
  switch (side) {
    case "top":
      return [
        [
          Math.min(triggerRect.left, contentRect.left) - buffer,
          triggerRect.top
        ],
        [
          Math.min(triggerRect.left, contentRect.left) - buffer,
          contentRect.bottom
        ],
        [
          Math.max(triggerRect.right, contentRect.right) + buffer,
          contentRect.bottom
        ],
        [
          Math.max(triggerRect.right, contentRect.right) + buffer,
          triggerRect.top
        ]
      ];
    case "bottom":
      return [
        [
          Math.min(triggerRect.left, contentRect.left) - buffer,
          triggerRect.bottom
        ],
        [
          Math.min(triggerRect.left, contentRect.left) - buffer,
          contentRect.top
        ],
        [
          Math.max(triggerRect.right, contentRect.right) + buffer,
          contentRect.top
        ],
        [
          Math.max(triggerRect.right, contentRect.right) + buffer,
          triggerRect.bottom
        ]
      ];
    case "left":
      return [
        [
          triggerRect.left,
          Math.min(triggerRect.top, contentRect.top) - buffer
        ],
        [
          contentRect.right,
          Math.min(triggerRect.top, contentRect.top) - buffer
        ],
        [
          contentRect.right,
          Math.max(triggerRect.bottom, contentRect.bottom) + buffer
        ],
        [
          triggerRect.left,
          Math.max(triggerRect.bottom, contentRect.bottom) + buffer
        ]
      ];
    case "right":
      return [
        [
          triggerRect.right,
          Math.min(triggerRect.top, contentRect.top) - buffer
        ],
        [
          contentRect.left,
          Math.min(triggerRect.top, contentRect.top) - buffer
        ],
        [
          contentRect.left,
          Math.max(triggerRect.bottom, contentRect.bottom) + buffer
        ],
        [
          triggerRect.right,
          Math.max(triggerRect.bottom, contentRect.bottom) + buffer
        ]
      ];
  }
};
/**
 * Creates a triangular/trapezoidal safe zone from the exit point to the target
 */
getSafePolygon_fn = function(exitPoint, targetRect, side, exitTarget) {
  const buffer = __privateGet(this, _buffer) * 4;
  const [x, y] = exitPoint;
  const effectiveSide = exitTarget === "trigger" ? __privateMethod(this, _SafePolygon_instances, flipSide_fn).call(this, side) : side;
  switch (effectiveSide) {
    case "top":
      return [
        [x - buffer, y + buffer],
        [x + buffer, y + buffer],
        [targetRect.right + buffer, targetRect.bottom],
        [targetRect.right + buffer, targetRect.top],
        [targetRect.left - buffer, targetRect.top],
        [targetRect.left - buffer, targetRect.bottom]
      ];
    case "bottom":
      return [
        [x - buffer, y - buffer],
        [x + buffer, y - buffer],
        [targetRect.right + buffer, targetRect.top],
        [targetRect.right + buffer, targetRect.bottom],
        [targetRect.left - buffer, targetRect.bottom],
        [targetRect.left - buffer, targetRect.top]
      ];
    case "left":
      return [
        [x + buffer, y - buffer],
        [x + buffer, y + buffer],
        [targetRect.right, targetRect.bottom + buffer],
        [targetRect.left, targetRect.bottom + buffer],
        [targetRect.left, targetRect.top - buffer],
        [targetRect.right, targetRect.top - buffer]
      ];
    case "right":
      return [
        [x - buffer, y - buffer],
        [x - buffer, y + buffer],
        [targetRect.left, targetRect.bottom + buffer],
        [targetRect.right, targetRect.bottom + buffer],
        [targetRect.right, targetRect.top - buffer],
        [targetRect.left, targetRect.top - buffer]
      ];
  }
};
flipSide_fn = function(side) {
  switch (side) {
    case "top":
      return "bottom";
    case "bottom":
      return "top";
    case "left":
      return "right";
    case "right":
      return "left";
  }
};
const popoverAttrs = createBitsAttrs({
  component: "popover",
  parts: ["root", "trigger", "content", "close", "overlay"]
});
const PopoverRootContext = new Context("Popover.Root");
const _PopoverRootState = class _PopoverRootState {
  constructor(opts) {
    __privateAdd(this, _PopoverRootState_instances);
    __publicField(this, "opts");
    __publicField(this, "contentNode", null);
    __publicField(this, "contentPresence");
    __publicField(this, "triggerNode", null);
    __publicField(this, "overlayNode", null);
    __publicField(this, "overlayPresence");
    // hover tracking state
    __publicField(this, "openedViaHover", false);
    __publicField(this, "hasInteractedWithContent", false);
    __publicField(this, "hoverCooldown", false);
    __publicField(this, "closeDelay", 0);
    __privateAdd(this, _closeTimeout, null);
    __privateAdd(this, _domContext, null);
    this.opts = opts;
    this.contentPresence = new PresenceManager({
      ref: boxWith(() => this.contentNode),
      open: this.opts.open,
      onComplete: () => {
        this.opts.onOpenChangeComplete.current(this.opts.open.current);
      }
    });
    this.overlayPresence = new PresenceManager({ ref: boxWith(() => this.overlayNode), open: this.opts.open });
    watch(() => this.opts.open.current, (isOpen) => {
      if (!isOpen) {
        this.openedViaHover = false;
        this.hasInteractedWithContent = false;
        __privateMethod(this, _PopoverRootState_instances, clearCloseTimeout_fn).call(this);
      }
    });
  }
  static create(opts) {
    return PopoverRootContext.set(new _PopoverRootState(opts));
  }
  setDomContext(ctx) {
    __privateSet(this, _domContext, ctx);
  }
  toggleOpen() {
    __privateMethod(this, _PopoverRootState_instances, clearCloseTimeout_fn).call(this);
    this.opts.open.current = !this.opts.open.current;
  }
  handleClose() {
    __privateMethod(this, _PopoverRootState_instances, clearCloseTimeout_fn).call(this);
    if (!this.opts.open.current) return;
    this.opts.open.current = false;
  }
  handleHoverOpen() {
    __privateMethod(this, _PopoverRootState_instances, clearCloseTimeout_fn).call(this);
    if (this.opts.open.current) return;
    this.openedViaHover = true;
    this.opts.open.current = true;
  }
  handleHoverClose() {
    if (!this.opts.open.current) return;
    if (this.openedViaHover && !this.hasInteractedWithContent) {
      this.opts.open.current = false;
    }
  }
  handleDelayedHoverClose() {
    if (!this.opts.open.current) return;
    if (!this.openedViaHover || this.hasInteractedWithContent) return;
    __privateMethod(this, _PopoverRootState_instances, clearCloseTimeout_fn).call(this);
    if (this.closeDelay <= 0) {
      this.opts.open.current = false;
    } else if (__privateGet(this, _domContext)) {
      __privateSet(this, _closeTimeout, __privateGet(this, _domContext).setTimeout(
        () => {
          if (this.openedViaHover && !this.hasInteractedWithContent) {
            this.opts.open.current = false;
          }
          __privateSet(this, _closeTimeout, null);
        },
        this.closeDelay
      ));
    }
  }
  cancelDelayedClose() {
    __privateMethod(this, _PopoverRootState_instances, clearCloseTimeout_fn).call(this);
  }
  markInteraction() {
    this.hasInteractedWithContent = true;
    __privateMethod(this, _PopoverRootState_instances, clearCloseTimeout_fn).call(this);
  }
};
_closeTimeout = new WeakMap();
_domContext = new WeakMap();
_PopoverRootState_instances = new WeakSet();
clearCloseTimeout_fn = function() {
  if (__privateGet(this, _closeTimeout) !== null && __privateGet(this, _domContext)) {
    __privateGet(this, _domContext).clearTimeout(__privateGet(this, _closeTimeout));
    __privateSet(this, _closeTimeout, null);
  }
};
let PopoverRootState = _PopoverRootState;
const _PopoverTriggerState = class _PopoverTriggerState {
  constructor(opts, root) {
    __privateAdd(this, _PopoverTriggerState_instances);
    __publicField(this, "opts");
    __publicField(this, "root");
    __publicField(this, "attachment");
    __publicField(this, "domContext");
    __privateAdd(this, _openTimeout, null);
    __privateAdd(this, _closeTimeout2, null);
    __privateAdd(this, _isHovering, false);
    __privateAdd(this, _props2, derived(() => ({
      id: this.opts.id.current,
      "aria-haspopup": "dialog",
      "aria-expanded": boolToStr(this.root.opts.open.current),
      "data-state": getDataOpenClosed(this.root.opts.open.current),
      "aria-controls": __privateMethod(this, _PopoverTriggerState_instances, getAriaControls_fn).call(this),
      [popoverAttrs.trigger]: "",
      disabled: this.opts.disabled.current,
      //
      onkeydown: this.onkeydown,
      onclick: this.onclick,
      onpointerenter: this.onpointerenter,
      onpointerleave: this.onpointerleave,
      ...this.attachment
    })));
    this.opts = opts;
    this.root = root;
    this.attachment = attachRef(this.opts.ref, (v) => this.root.triggerNode = v);
    this.domContext = new DOMContext(opts.ref);
    this.root.setDomContext(this.domContext);
    this.onclick = this.onclick.bind(this);
    this.onkeydown = this.onkeydown.bind(this);
    this.onpointerenter = this.onpointerenter.bind(this);
    this.onpointerleave = this.onpointerleave.bind(this);
    watch(() => this.opts.closeDelay.current, (delay) => {
      this.root.closeDelay = delay;
    });
  }
  static create(opts) {
    return new _PopoverTriggerState(opts, PopoverRootContext.get());
  }
  onpointerenter(e) {
    if (this.opts.disabled.current) return;
    if (!this.opts.openOnHover.current) return;
    if (isTouch(e)) return;
    __privateSet(this, _isHovering, true);
    __privateMethod(this, _PopoverTriggerState_instances, clearCloseTimeout_fn2).call(this);
    this.root.cancelDelayedClose();
    if (this.root.opts.open.current || this.root.hoverCooldown) return;
    const delay = this.opts.openDelay.current;
    if (delay <= 0) {
      this.root.handleHoverOpen();
    } else {
      __privateSet(this, _openTimeout, this.domContext.setTimeout(
        () => {
          this.root.handleHoverOpen();
          __privateSet(this, _openTimeout, null);
        },
        delay
      ));
    }
  }
  onpointerleave(e) {
    if (this.opts.disabled.current) return;
    if (!this.opts.openOnHover.current) return;
    if (isTouch(e)) return;
    __privateSet(this, _isHovering, false);
    __privateMethod(this, _PopoverTriggerState_instances, clearOpenTimeout_fn).call(this);
    this.root.hoverCooldown = false;
  }
  onclick(e) {
    if (this.opts.disabled.current) return;
    if (e.button !== 0) return;
    __privateMethod(this, _PopoverTriggerState_instances, clearAllTimeouts_fn).call(this);
    if (__privateGet(this, _isHovering) && this.root.opts.open.current && this.root.openedViaHover) {
      this.root.openedViaHover = false;
      this.root.hasInteractedWithContent = true;
      return;
    }
    if (__privateGet(this, _isHovering) && this.opts.openOnHover.current && this.root.opts.open.current) {
      this.root.hoverCooldown = true;
    }
    if (this.root.hoverCooldown && !this.root.opts.open.current) {
      this.root.hoverCooldown = false;
    }
    this.root.toggleOpen();
  }
  onkeydown(e) {
    if (this.opts.disabled.current) return;
    if (!(e.key === ENTER || e.key === SPACE)) return;
    e.preventDefault();
    __privateMethod(this, _PopoverTriggerState_instances, clearAllTimeouts_fn).call(this);
    this.root.toggleOpen();
  }
  get props() {
    return __privateGet(this, _props2).call(this);
  }
  set props($$value) {
    return __privateGet(this, _props2).call(this, $$value);
  }
};
_openTimeout = new WeakMap();
_closeTimeout2 = new WeakMap();
_isHovering = new WeakMap();
_PopoverTriggerState_instances = new WeakSet();
clearOpenTimeout_fn = function() {
  if (__privateGet(this, _openTimeout) !== null) {
    this.domContext.clearTimeout(__privateGet(this, _openTimeout));
    __privateSet(this, _openTimeout, null);
  }
};
clearCloseTimeout_fn2 = function() {
  if (__privateGet(this, _closeTimeout2) !== null) {
    this.domContext.clearTimeout(__privateGet(this, _closeTimeout2));
    __privateSet(this, _closeTimeout2, null);
  }
};
clearAllTimeouts_fn = function() {
  __privateMethod(this, _PopoverTriggerState_instances, clearOpenTimeout_fn).call(this);
  __privateMethod(this, _PopoverTriggerState_instances, clearCloseTimeout_fn2).call(this);
};
getAriaControls_fn = function() {
  var _a, _b;
  if (this.root.opts.open.current && ((_a = this.root.contentNode) == null ? void 0 : _a.id)) {
    return (_b = this.root.contentNode) == null ? void 0 : _b.id;
  }
};
_props2 = new WeakMap();
let PopoverTriggerState = _PopoverTriggerState;
const _PopoverContentState = class _PopoverContentState {
  constructor(opts, root) {
    __publicField(this, "opts");
    __publicField(this, "root");
    __publicField(this, "attachment");
    __publicField(this, "onInteractOutside", (e) => {
      this.opts.onInteractOutside.current(e);
      if (e.defaultPrevented) return;
      if (!isElement$1(e.target)) return;
      const closestTrigger = e.target.closest(popoverAttrs.selector("trigger"));
      if (closestTrigger && closestTrigger === this.root.triggerNode) return;
      if (this.opts.customAnchor.current) {
        if (isElement$1(this.opts.customAnchor.current)) {
          if (this.opts.customAnchor.current.contains(e.target)) return;
        } else if (typeof this.opts.customAnchor.current === "string") {
          const el = document.querySelector(this.opts.customAnchor.current);
          if (el && el.contains(e.target)) return;
        }
      }
      this.root.handleClose();
    });
    __publicField(this, "onEscapeKeydown", (e) => {
      this.opts.onEscapeKeydown.current(e);
      if (e.defaultPrevented) return;
      this.root.handleClose();
    });
    __privateAdd(this, _snippetProps, derived(() => ({ open: this.root.opts.open.current })));
    __privateAdd(this, _props3, derived(() => ({
      id: this.opts.id.current,
      tabindex: -1,
      "data-state": getDataOpenClosed(this.root.opts.open.current),
      ...getDataTransitionAttrs(this.root.contentPresence.transitionStatus),
      [popoverAttrs.content]: "",
      style: { pointerEvents: "auto", contain: "layout style" },
      onpointerdown: this.onpointerdown,
      onfocusin: this.onfocusin,
      onpointerenter: this.onpointerenter,
      onpointerleave: this.onpointerleave,
      ...this.attachment
    })));
    __publicField(this, "popperProps", {
      onInteractOutside: this.onInteractOutside,
      onEscapeKeydown: this.onEscapeKeydown
    });
    this.opts = opts;
    this.root = root;
    this.attachment = attachRef(this.opts.ref, (v) => this.root.contentNode = v);
    this.onpointerdown = this.onpointerdown.bind(this);
    this.onfocusin = this.onfocusin.bind(this);
    this.onpointerenter = this.onpointerenter.bind(this);
    this.onpointerleave = this.onpointerleave.bind(this);
    new SafePolygon({
      triggerNode: () => this.root.triggerNode,
      contentNode: () => this.root.contentNode,
      enabled: () => this.root.opts.open.current && this.root.openedViaHover && !this.root.hasInteractedWithContent,
      onPointerExit: () => {
        this.root.handleDelayedHoverClose();
      }
    });
  }
  static create(opts) {
    return new _PopoverContentState(opts, PopoverRootContext.get());
  }
  onpointerdown(_) {
    this.root.markInteraction();
  }
  onfocusin(e) {
    const target = e.target;
    if (isElement$1(target) && isTabbable(target)) {
      this.root.markInteraction();
    }
  }
  onpointerenter(e) {
    if (isTouch(e)) return;
    this.root.cancelDelayedClose();
  }
  onpointerleave(e) {
    if (isTouch(e)) return;
  }
  get shouldRender() {
    return this.root.contentPresence.shouldRender;
  }
  get shouldTrapFocus() {
    if (this.root.openedViaHover && !this.root.hasInteractedWithContent) return false;
    return true;
  }
  get snippetProps() {
    return __privateGet(this, _snippetProps).call(this);
  }
  set snippetProps($$value) {
    return __privateGet(this, _snippetProps).call(this, $$value);
  }
  get props() {
    return __privateGet(this, _props3).call(this);
  }
  set props($$value) {
    return __privateGet(this, _props3).call(this, $$value);
  }
};
_snippetProps = new WeakMap();
_props3 = new WeakMap();
let PopoverContentState = _PopoverContentState;
function Popover_content($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    const uid = props_id($$renderer2);
    let {
      child,
      children,
      ref = null,
      id = createId(uid),
      forceMount = false,
      onOpenAutoFocus = noop,
      onCloseAutoFocus = noop,
      onEscapeKeydown = noop,
      onInteractOutside = noop,
      trapFocus = true,
      preventScroll = false,
      customAnchor = null,
      style,
      $$slots,
      $$events,
      ...restProps
    } = $$props;
    const contentState = PopoverContentState.create({
      id: boxWith(() => id),
      ref: boxWith(() => ref, (v) => ref = v),
      onInteractOutside: boxWith(() => onInteractOutside),
      onEscapeKeydown: boxWith(() => onEscapeKeydown),
      customAnchor: boxWith(() => customAnchor)
    });
    const mergedProps = derived(() => mergeProps(restProps, contentState.props));
    const effectiveTrapFocus = derived(() => trapFocus && contentState.shouldTrapFocus);
    function handleOpenAutoFocus(e) {
      if (!contentState.shouldTrapFocus) {
        e.preventDefault();
      }
      onOpenAutoFocus(e);
    }
    if (forceMount) {
      $$renderer2.push("<!--[0-->");
      {
        let popper = function($$renderer3, { props, wrapperProps }) {
          const finalProps = mergeProps(props, { style: getFloatingContentCSSVars("popover") }, { style });
          if (child) {
            $$renderer3.push("<!--[0-->");
            child($$renderer3, {
              props: finalProps,
              wrapperProps,
              ...contentState.snippetProps
            });
            $$renderer3.push(`<!---->`);
          } else {
            $$renderer3.push(`<!--[-1--><div${attributes({ ...wrapperProps })}><div${attributes({ ...finalProps })}>`);
            children == null ? void 0 : children($$renderer3);
            $$renderer3.push(`<!----></div></div>`);
          }
          $$renderer3.push(`<!--]-->`);
        };
        Popper_layer_force_mount($$renderer2, spread_props([
          mergedProps(),
          contentState.popperProps,
          {
            ref: contentState.opts.ref,
            enabled: contentState.root.opts.open.current,
            id,
            trapFocus: effectiveTrapFocus(),
            preventScroll,
            loop: true,
            forceMount: true,
            customAnchor,
            onOpenAutoFocus: handleOpenAutoFocus,
            onCloseAutoFocus,
            shouldRender: contentState.shouldRender,
            popper,
            $$slots: { popper: true }
          }
        ]));
      }
    } else if (!forceMount) {
      $$renderer2.push("<!--[1-->");
      {
        let popper = function($$renderer3, { props, wrapperProps }) {
          const finalProps = mergeProps(props, { style: getFloatingContentCSSVars("popover") }, { style });
          if (child) {
            $$renderer3.push("<!--[0-->");
            child($$renderer3, {
              props: finalProps,
              wrapperProps,
              ...contentState.snippetProps
            });
            $$renderer3.push(`<!---->`);
          } else {
            $$renderer3.push(`<!--[-1--><div${attributes({ ...wrapperProps })}><div${attributes({ ...finalProps })}>`);
            children == null ? void 0 : children($$renderer3);
            $$renderer3.push(`<!----></div></div>`);
          }
          $$renderer3.push(`<!--]-->`);
        };
        Popper_layer($$renderer2, spread_props([
          mergedProps(),
          contentState.popperProps,
          {
            ref: contentState.opts.ref,
            open: contentState.root.opts.open.current,
            id,
            trapFocus: effectiveTrapFocus(),
            preventScroll,
            loop: true,
            forceMount: false,
            customAnchor,
            onOpenAutoFocus: handleOpenAutoFocus,
            onCloseAutoFocus,
            shouldRender: contentState.shouldRender,
            popper,
            $$slots: { popper: true }
          }
        ]));
      }
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]-->`);
    bind_props($$props, { ref });
  });
}
function Popover_trigger($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    const uid = props_id($$renderer2);
    let {
      children,
      child,
      id = createId(uid),
      ref = null,
      type = "button",
      disabled = false,
      openOnHover = false,
      openDelay = 700,
      closeDelay = 300,
      $$slots,
      $$events,
      ...restProps
    } = $$props;
    const triggerState = PopoverTriggerState.create({
      id: boxWith(() => id),
      ref: boxWith(() => ref, (v) => ref = v),
      disabled: boxWith(() => Boolean(disabled)),
      openOnHover: boxWith(() => openOnHover),
      openDelay: boxWith(() => openDelay),
      closeDelay: boxWith(() => closeDelay)
    });
    const mergedProps = derived(() => mergeProps(restProps, triggerState.props, { type }));
    Floating_layer_anchor($$renderer2, {
      id,
      ref: triggerState.opts.ref,
      children: ($$renderer3) => {
        if (child) {
          $$renderer3.push("<!--[0-->");
          child($$renderer3, { props: mergedProps() });
          $$renderer3.push(`<!---->`);
        } else {
          $$renderer3.push(`<!--[-1--><button${attributes({ ...mergedProps() })}>`);
          children == null ? void 0 : children($$renderer3);
          $$renderer3.push(`<!----></button>`);
        }
        $$renderer3.push(`<!--]-->`);
      }
    });
    bind_props($$props, { ref });
  });
}
function Popover($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      open = false,
      onOpenChange = noop,
      onOpenChangeComplete = noop,
      children
    } = $$props;
    PopoverRootState.create({
      open: boxWith(() => open, (v) => {
        open = v;
        onOpenChange(v);
      }),
      onOpenChangeComplete: boxWith(() => onOpenChangeComplete)
    });
    Floating_layer($$renderer2, {
      children: ($$renderer3) => {
        children == null ? void 0 : children($$renderer3);
        $$renderer3.push(`<!---->`);
      }
    });
    bind_props($$props, { open });
  });
}
class TimeoutFn {
  constructor(cb, interval) {
    __privateAdd(this, _TimeoutFn_instances);
    __privateAdd(this, _interval);
    __privateAdd(this, _cb);
    __privateAdd(this, _timer, null);
    __privateSet(this, _cb, cb);
    __privateSet(this, _interval, interval);
    this.stop = this.stop.bind(this);
    this.start = this.start.bind(this);
    onDestroyEffect(this.stop);
  }
  stop() {
    __privateMethod(this, _TimeoutFn_instances, clear_fn).call(this);
  }
  start(...args) {
    __privateMethod(this, _TimeoutFn_instances, clear_fn).call(this);
    __privateSet(this, _timer, window.setTimeout(() => {
      __privateSet(this, _timer, null);
      __privateGet(this, _cb).call(this, ...args);
    }, __privateGet(this, _interval)));
  }
}
_interval = new WeakMap();
_cb = new WeakMap();
_timer = new WeakMap();
_TimeoutFn_instances = new WeakSet();
clear_fn = function() {
  if (__privateGet(this, _timer) !== null) {
    window.clearTimeout(__privateGet(this, _timer));
    __privateSet(this, _timer, null);
  }
};
const tooltipAttrs = createBitsAttrs({ component: "tooltip", parts: ["content", "trigger"] });
const TooltipProviderContext = new Context("Tooltip.Provider");
const TooltipRootContext = new Context("Tooltip.Root");
class TooltipTriggerRegistryState {
  constructor() {
    __publicField(this, "triggers", /* @__PURE__ */ new Map());
    __publicField(this, "activeTriggerId", null);
    __privateAdd(this, _activeTriggerNode, derived(() => {
      var _a;
      const activeTriggerId = this.activeTriggerId;
      if (activeTriggerId === null) return null;
      return ((_a = this.triggers.get(activeTriggerId)) == null ? void 0 : _a.node) ?? null;
    }));
    __privateAdd(this, _activePayload, derived(() => {
      var _a;
      const activeTriggerId = this.activeTriggerId;
      if (activeTriggerId === null) return null;
      return ((_a = this.triggers.get(activeTriggerId)) == null ? void 0 : _a.payload) ?? null;
    }));
    __publicField(this, "register", (record) => {
      const next = new Map(this.triggers);
      next.set(record.id, record);
      this.triggers = next;
      __privateGet(this, _coerceActiveTrigger).call(this);
    });
    __publicField(this, "update", (record) => {
      const next = new Map(this.triggers);
      next.set(record.id, record);
      this.triggers = next;
      __privateGet(this, _coerceActiveTrigger).call(this);
    });
    __publicField(this, "unregister", (id) => {
      if (!this.triggers.has(id)) return;
      const next = new Map(this.triggers);
      next.delete(id);
      this.triggers = next;
      if (this.activeTriggerId === id) {
        this.activeTriggerId = null;
      }
    });
    __publicField(this, "setActiveTrigger", (id) => {
      if (id === null) {
        this.activeTriggerId = null;
        return;
      }
      if (!this.triggers.has(id)) {
        this.activeTriggerId = null;
        return;
      }
      this.activeTriggerId = id;
    });
    __publicField(this, "get", (id) => {
      return this.triggers.get(id);
    });
    __publicField(this, "has", (id) => {
      return this.triggers.has(id);
    });
    __publicField(this, "getFirstTriggerId", () => {
      const firstEntry = this.triggers.entries().next();
      if (firstEntry.done) return null;
      return firstEntry.value[0];
    });
    __privateAdd(this, _coerceActiveTrigger, () => {
      const activeTriggerId = this.activeTriggerId;
      if (activeTriggerId === null) return;
      if (!this.triggers.has(activeTriggerId)) {
        this.activeTriggerId = null;
      }
    });
  }
  get activeTriggerNode() {
    return __privateGet(this, _activeTriggerNode).call(this);
  }
  set activeTriggerNode($$value) {
    return __privateGet(this, _activeTriggerNode).call(this, $$value);
  }
  get activePayload() {
    return __privateGet(this, _activePayload).call(this);
  }
  set activePayload($$value) {
    return __privateGet(this, _activePayload).call(this, $$value);
  }
}
_activeTriggerNode = new WeakMap();
_activePayload = new WeakMap();
_coerceActiveTrigger = new WeakMap();
const _TooltipProviderState = class _TooltipProviderState {
  constructor(opts) {
    __publicField(this, "opts");
    __publicField(this, "isOpenDelayed", true);
    __publicField(this, "isPointerInTransit", simpleBox(false));
    __privateAdd(this, _timerFn);
    __privateAdd(this, _openTooltip, null);
    __privateAdd(this, _startTimer, () => {
      const skipDuration = this.opts.skipDelayDuration.current;
      if (skipDuration === 0) {
        this.isOpenDelayed = true;
        return;
      } else {
        __privateGet(this, _timerFn).start();
      }
    });
    __privateAdd(this, _clearTimer, () => {
      __privateGet(this, _timerFn).stop();
    });
    __publicField(this, "onOpen", (tooltip) => {
      if (__privateGet(this, _openTooltip) && __privateGet(this, _openTooltip) !== tooltip) {
        __privateGet(this, _openTooltip).handleClose();
      }
      __privateGet(this, _clearTimer).call(this);
      this.isOpenDelayed = false;
      __privateSet(this, _openTooltip, tooltip);
    });
    __publicField(this, "onClose", (tooltip) => {
      if (__privateGet(this, _openTooltip) === tooltip) {
        __privateSet(this, _openTooltip, null);
        __privateGet(this, _startTimer).call(this);
      }
    });
    __publicField(this, "isTooltipOpen", (tooltip) => {
      return __privateGet(this, _openTooltip) === tooltip;
    });
    this.opts = opts;
    __privateSet(this, _timerFn, new TimeoutFn(
      () => {
        this.isOpenDelayed = true;
      },
      this.opts.skipDelayDuration.current
    ));
  }
  static create(opts) {
    return TooltipProviderContext.set(new _TooltipProviderState(opts));
  }
};
_timerFn = new WeakMap();
_openTooltip = new WeakMap();
_startTimer = new WeakMap();
_clearTimer = new WeakMap();
let TooltipProviderState = _TooltipProviderState;
const _TooltipRootState = class _TooltipRootState {
  constructor(opts, provider) {
    __publicField(this, "opts");
    __publicField(this, "provider");
    __privateAdd(this, _delayDuration, derived(() => this.opts.delayDuration.current ?? this.provider.opts.delayDuration.current));
    __privateAdd(this, _disableHoverableContent, derived(() => this.opts.disableHoverableContent.current ?? this.provider.opts.disableHoverableContent.current));
    __privateAdd(this, _disableCloseOnTriggerClick, derived(() => this.opts.disableCloseOnTriggerClick.current ?? this.provider.opts.disableCloseOnTriggerClick.current));
    __privateAdd(this, _disabled, derived(() => this.opts.disabled.current ?? this.provider.opts.disabled.current));
    __privateAdd(this, _ignoreNonKeyboardFocus, derived(() => this.opts.ignoreNonKeyboardFocus.current ?? this.provider.opts.ignoreNonKeyboardFocus.current));
    __publicField(this, "registry");
    __publicField(this, "tether");
    __publicField(this, "contentNode", null);
    __publicField(this, "contentPresence");
    __privateAdd(this, _wasOpenDelayed, false);
    __privateAdd(this, _timerFn2);
    __privateAdd(this, _stateAttr, derived(() => {
      if (!this.opts.open.current) return "closed";
      return __privateGet(this, _wasOpenDelayed) ? "delayed-open" : "instant-open";
    }));
    __publicField(this, "handleOpen", () => {
      __privateGet(this, _timerFn2).stop();
      __privateSet(this, _wasOpenDelayed, false);
      this.ensureActiveTrigger();
      this.opts.open.current = true;
    });
    __publicField(this, "handleClose", () => {
      __privateGet(this, _timerFn2).stop();
      this.opts.open.current = false;
    });
    /** Stops a pending delayed open without touching the open state. */
    __publicField(this, "cancelPendingOpen", () => {
      __privateGet(this, _timerFn2).stop();
    });
    __privateAdd(this, _handleDelayedOpen, () => {
      __privateGet(this, _timerFn2).stop();
      const shouldSkipDelay = !this.provider.isOpenDelayed;
      const delayDuration = this.delayDuration ?? 0;
      if (shouldSkipDelay || delayDuration === 0) {
        __privateSet(this, _wasOpenDelayed, false);
        this.opts.open.current = true;
      } else {
        __privateGet(this, _timerFn2).start();
      }
    });
    __publicField(this, "onTriggerEnter", (triggerId) => {
      this.setActiveTrigger(triggerId);
      __privateGet(this, _handleDelayedOpen).call(this);
    });
    __publicField(this, "onTriggerLeave", () => {
      if (this.disableHoverableContent) {
        this.handleClose();
      } else {
        __privateGet(this, _timerFn2).stop();
      }
    });
    __publicField(this, "ensureActiveTrigger", () => {
      if (this.registry.activeTriggerId !== null && this.registry.has(this.registry.activeTriggerId)) {
        return;
      }
      if (this.opts.triggerId.current !== null && this.registry.has(this.opts.triggerId.current)) {
        this.registry.setActiveTrigger(this.opts.triggerId.current);
        return;
      }
      const firstTriggerId = this.registry.getFirstTriggerId();
      this.registry.setActiveTrigger(firstTriggerId);
    });
    __publicField(this, "setActiveTrigger", (triggerId) => {
      this.registry.setActiveTrigger(triggerId);
    });
    __publicField(this, "registerTrigger", (trigger) => {
      this.registry.register(trigger);
      if (trigger.disabled && this.registry.activeTriggerId === trigger.id && this.opts.open.current) {
        this.handleClose();
      }
    });
    __publicField(this, "updateTrigger", (trigger) => {
      this.registry.update(trigger);
      if (trigger.disabled && this.registry.activeTriggerId === trigger.id && this.opts.open.current) {
        this.handleClose();
      }
    });
    __publicField(this, "unregisterTrigger", (id) => {
      const isActive = this.registry.activeTriggerId === id;
      this.registry.unregister(id);
      if (isActive && this.opts.open.current) {
        this.handleClose();
      }
    });
    __publicField(this, "isActiveTrigger", (triggerId) => {
      return this.registry.activeTriggerId === triggerId;
    });
    var _a, _b;
    this.opts = opts;
    this.provider = provider;
    this.tether = ((_a = opts.tether.current) == null ? void 0 : _a.state) ?? null;
    this.registry = ((_b = this.tether) == null ? void 0 : _b.registry) ?? new TooltipTriggerRegistryState();
    __privateSet(this, _timerFn2, new TimeoutFn(
      () => {
        __privateSet(this, _wasOpenDelayed, true);
        this.opts.open.current = true;
      },
      this.delayDuration ?? 0
    ));
    if (this.tether) {
      this.tether.root = this;
    }
    this.contentPresence = new PresenceManager({
      open: this.opts.open,
      ref: boxWith(() => this.contentNode),
      onComplete: () => {
        this.opts.onOpenChangeComplete.current(this.opts.open.current);
      }
    });
    watch(() => this.delayDuration, () => {
      if (this.delayDuration === void 0) return;
      __privateSet(this, _timerFn2, new TimeoutFn(
        () => {
          __privateSet(this, _wasOpenDelayed, true);
          this.opts.open.current = true;
        },
        this.delayDuration
      ));
    });
    watch(
      () => this.opts.open.current,
      (isOpen) => {
        if (isOpen) {
          this.ensureActiveTrigger();
          this.provider.onOpen(this);
        } else {
          this.provider.onClose(this);
        }
      },
      { lazy: true }
    );
    watch(() => this.opts.triggerId.current, (triggerId) => {
      if (triggerId === this.registry.activeTriggerId) return;
      this.registry.setActiveTrigger(triggerId);
    });
    watch(() => this.registry.activeTriggerId, (activeTriggerId) => {
      if (this.opts.triggerId.current === activeTriggerId) return;
      this.opts.triggerId.current = activeTriggerId;
    });
  }
  static create(opts) {
    return TooltipRootContext.set(new _TooltipRootState(opts, TooltipProviderContext.get()));
  }
  get delayDuration() {
    return __privateGet(this, _delayDuration).call(this);
  }
  set delayDuration($$value) {
    return __privateGet(this, _delayDuration).call(this, $$value);
  }
  get disableHoverableContent() {
    return __privateGet(this, _disableHoverableContent).call(this);
  }
  set disableHoverableContent($$value) {
    return __privateGet(this, _disableHoverableContent).call(this, $$value);
  }
  get disableCloseOnTriggerClick() {
    return __privateGet(this, _disableCloseOnTriggerClick).call(this);
  }
  set disableCloseOnTriggerClick($$value) {
    return __privateGet(this, _disableCloseOnTriggerClick).call(this, $$value);
  }
  get disabled() {
    return __privateGet(this, _disabled).call(this);
  }
  set disabled($$value) {
    return __privateGet(this, _disabled).call(this, $$value);
  }
  get ignoreNonKeyboardFocus() {
    return __privateGet(this, _ignoreNonKeyboardFocus).call(this);
  }
  set ignoreNonKeyboardFocus($$value) {
    return __privateGet(this, _ignoreNonKeyboardFocus).call(this, $$value);
  }
  get stateAttr() {
    return __privateGet(this, _stateAttr).call(this);
  }
  set stateAttr($$value) {
    return __privateGet(this, _stateAttr).call(this, $$value);
  }
  get triggerNode() {
    return this.registry.activeTriggerNode;
  }
  get activePayload() {
    return this.registry.activePayload;
  }
  get activeTriggerId() {
    return this.registry.activeTriggerId;
  }
};
_delayDuration = new WeakMap();
_disableHoverableContent = new WeakMap();
_disableCloseOnTriggerClick = new WeakMap();
_disabled = new WeakMap();
_ignoreNonKeyboardFocus = new WeakMap();
_wasOpenDelayed = new WeakMap();
_timerFn2 = new WeakMap();
_stateAttr = new WeakMap();
_handleDelayedOpen = new WeakMap();
let TooltipRootState = _TooltipRootState;
const _TooltipTriggerState = class _TooltipTriggerState {
  constructor(opts, root, tether) {
    __publicField(this, "opts");
    __publicField(this, "root");
    __publicField(this, "tether");
    __publicField(this, "attachment");
    __privateAdd(this, _isPointerDown, simpleBox(false));
    __privateAdd(this, _hasPointerMoveOpened, false);
    __publicField(this, "domContext");
    __privateAdd(this, _transitCheckTimeout, null);
    __privateAdd(this, _mounted, false);
    __privateAdd(this, _lastRegisteredId, null);
    __privateAdd(this, _getRoot, () => {
      var _a;
      return ((_a = this.tether) == null ? void 0 : _a.root) ?? this.root;
    });
    __privateAdd(this, _isDisabled, () => {
      const root = __privateGet(this, _getRoot).call(this);
      return this.opts.disabled.current || Boolean(root == null ? void 0 : root.disabled);
    });
    __privateAdd(this, _register, (node) => {
      if (!__privateGet(this, _mounted)) return;
      const id = this.opts.id.current;
      const payload = this.opts.payload.current;
      const disabled = this.opts.disabled.current;
      if (__privateGet(this, _lastRegisteredId) && __privateGet(this, _lastRegisteredId) !== id) {
        const root2 = __privateGet(this, _getRoot).call(this);
        if (this.tether) {
          this.tether.registry.unregister(__privateGet(this, _lastRegisteredId));
        } else {
          root2 == null ? void 0 : root2.unregisterTrigger(__privateGet(this, _lastRegisteredId));
        }
      }
      const triggerRecord = { id, node, payload, disabled };
      const root = __privateGet(this, _getRoot).call(this);
      if (this.tether) {
        if (this.tether.registry.has(id)) {
          this.tether.registry.update(triggerRecord);
        } else {
          this.tether.registry.register(triggerRecord);
        }
        if (disabled && this.tether.registry.activeTriggerId === id && (root == null ? void 0 : root.opts.open.current)) {
          root.handleClose();
        }
      } else {
        if (root == null ? void 0 : root.registry.has(id)) {
          root.updateTrigger(triggerRecord);
        } else {
          root == null ? void 0 : root.registerTrigger(triggerRecord);
        }
      }
      __privateSet(this, _lastRegisteredId, id);
    });
    __privateAdd(this, _clearTransitCheck, () => {
      if (__privateGet(this, _transitCheckTimeout) !== null) {
        clearTimeout(__privateGet(this, _transitCheckTimeout));
        __privateSet(this, _transitCheckTimeout, null);
      }
    });
    __publicField(this, "handlePointerUp", () => {
      __privateGet(this, _isPointerDown).current = false;
    });
    __privateAdd(this, _onpointerup, () => {
      if (__privateGet(this, _isDisabled).call(this)) return;
      __privateGet(this, _isPointerDown).current = false;
    });
    __privateAdd(this, _onpointerdown, () => {
      if (__privateGet(this, _isDisabled).call(this)) return;
      const root = __privateGet(this, _getRoot).call(this);
      if (root && !root.disableCloseOnTriggerClick) {
        if (root.opts.open.current) {
          root.handleClose();
        } else {
          root.cancelPendingOpen();
        }
      }
      __privateGet(this, _isPointerDown).current = true;
      this.domContext.getDocument().addEventListener(
        "pointerup",
        () => {
          this.handlePointerUp();
        },
        { once: true }
      );
    });
    __privateAdd(this, _onpointerenter, (e) => {
      const root = __privateGet(this, _getRoot).call(this);
      if (!root) return;
      if (__privateGet(this, _isDisabled).call(this)) {
        if (root.opts.open.current) {
          root.handleClose();
        }
        return;
      }
      if (e.pointerType === "touch") return;
      if (root.provider.isPointerInTransit.current) {
        __privateGet(this, _clearTransitCheck).call(this);
        __privateSet(this, _transitCheckTimeout, window.setTimeout(
          () => {
            if (root.provider.isPointerInTransit.current) {
              root.provider.isPointerInTransit.current = false;
              root.onTriggerEnter(this.opts.id.current);
              __privateSet(this, _hasPointerMoveOpened, true);
            }
          },
          250
        ));
        return;
      }
      root.onTriggerEnter(this.opts.id.current);
      __privateSet(this, _hasPointerMoveOpened, true);
    });
    __privateAdd(this, _onpointermove, (e) => {
      const root = __privateGet(this, _getRoot).call(this);
      if (!root) return;
      if (__privateGet(this, _isDisabled).call(this)) {
        if (root.opts.open.current) {
          root.handleClose();
        }
        return;
      }
      if (e.pointerType === "touch") return;
      if (__privateGet(this, _hasPointerMoveOpened)) return;
      __privateGet(this, _clearTransitCheck).call(this);
      root.provider.isPointerInTransit.current = false;
      root.onTriggerEnter(this.opts.id.current);
      __privateSet(this, _hasPointerMoveOpened, true);
    });
    __privateAdd(this, _onpointerleave, (e) => {
      const root = __privateGet(this, _getRoot).call(this);
      if (!root) return;
      if (__privateGet(this, _isDisabled).call(this)) return;
      __privateGet(this, _clearTransitCheck).call(this);
      if (!root.isActiveTrigger(this.opts.id.current)) {
        __privateSet(this, _hasPointerMoveOpened, false);
        return;
      }
      const relatedTarget = e.relatedTarget;
      if (isElement$1(relatedTarget)) {
        for (const record of root.registry.triggers.values()) {
          if (record.node !== relatedTarget) continue;
          if (root.provider.opts.skipDelayDuration.current > 0) {
            __privateSet(this, _hasPointerMoveOpened, false);
            return;
          }
          root.handleClose();
          __privateSet(this, _hasPointerMoveOpened, false);
          return;
        }
      }
      root.onTriggerLeave();
      __privateSet(this, _hasPointerMoveOpened, false);
    });
    __privateAdd(this, _onfocus, (e) => {
      const root = __privateGet(this, _getRoot).call(this);
      if (!root) return;
      if (__privateGet(this, _isPointerDown).current) return;
      if (__privateGet(this, _isDisabled).call(this)) {
        if (root.opts.open.current) {
          root.handleClose();
        }
        return;
      }
      if (root.ignoreNonKeyboardFocus && !isFocusVisible(e.currentTarget)) return;
      root.setActiveTrigger(this.opts.id.current);
      root.handleOpen();
    });
    __privateAdd(this, _onblur, () => {
      const root = __privateGet(this, _getRoot).call(this);
      if (!root || __privateGet(this, _isDisabled).call(this)) return;
      root.handleClose();
    });
    __privateAdd(this, _onclick, () => {
      const root = __privateGet(this, _getRoot).call(this);
      if (!root || root.disableCloseOnTriggerClick || __privateGet(this, _isDisabled).call(this)) return;
      root.handleClose();
    });
    __privateAdd(this, _props4, derived(() => {
      var _a;
      const root = __privateGet(this, _getRoot).call(this);
      const isOpenForTrigger = Boolean((root == null ? void 0 : root.opts.open.current) && root.isActiveTrigger(this.opts.id.current));
      const isDisabled = __privateGet(this, _isDisabled).call(this);
      return {
        id: this.opts.id.current,
        "aria-describedby": isOpenForTrigger ? (_a = root == null ? void 0 : root.contentNode) == null ? void 0 : _a.id : void 0,
        "data-state": isOpenForTrigger ? root == null ? void 0 : root.stateAttr : "closed",
        "data-disabled": boolToEmptyStrOrUndef(isDisabled),
        "data-delay-duration": `${(root == null ? void 0 : root.delayDuration) ?? 0}`,
        [tooltipAttrs.trigger]: "",
        tabindex: isDisabled ? void 0 : this.opts.tabindex.current,
        disabled: this.opts.disabled.current,
        onpointerup: __privateGet(this, _onpointerup),
        onpointerdown: __privateGet(this, _onpointerdown),
        onpointerenter: __privateGet(this, _onpointerenter),
        onpointermove: __privateGet(this, _onpointermove),
        onpointerleave: __privateGet(this, _onpointerleave),
        onfocus: __privateGet(this, _onfocus),
        onblur: __privateGet(this, _onblur),
        onclick: __privateGet(this, _onclick),
        ...this.attachment
      };
    }));
    this.opts = opts;
    this.root = root;
    this.tether = tether;
    this.domContext = new DOMContext(opts.ref);
    this.attachment = attachRef(this.opts.ref, (v) => __privateGet(this, _register).call(this, v));
    watch(() => this.opts.id.current, () => {
      __privateGet(this, _register).call(this, this.opts.ref.current);
    });
    watch(() => this.opts.payload.current, () => {
      __privateGet(this, _register).call(this, this.opts.ref.current);
    });
    watch(() => this.opts.disabled.current, () => {
      __privateGet(this, _register).call(this, this.opts.ref.current);
    });
  }
  static create(opts) {
    if (opts.tether.current) {
      return new _TooltipTriggerState(opts, null, opts.tether.current.state);
    }
    return new _TooltipTriggerState(opts, TooltipRootContext.get(), null);
  }
  get props() {
    return __privateGet(this, _props4).call(this);
  }
  set props($$value) {
    return __privateGet(this, _props4).call(this, $$value);
  }
};
_isPointerDown = new WeakMap();
_hasPointerMoveOpened = new WeakMap();
_transitCheckTimeout = new WeakMap();
_mounted = new WeakMap();
_lastRegisteredId = new WeakMap();
_getRoot = new WeakMap();
_isDisabled = new WeakMap();
_register = new WeakMap();
_clearTransitCheck = new WeakMap();
_onpointerup = new WeakMap();
_onpointerdown = new WeakMap();
_onpointerenter = new WeakMap();
_onpointermove = new WeakMap();
_onpointerleave = new WeakMap();
_onfocus = new WeakMap();
_onblur = new WeakMap();
_onclick = new WeakMap();
_props4 = new WeakMap();
let TooltipTriggerState = _TooltipTriggerState;
const _TooltipContentState = class _TooltipContentState {
  constructor(opts, root) {
    __publicField(this, "opts");
    __publicField(this, "root");
    __publicField(this, "attachment");
    __publicField(this, "onInteractOutside", (e) => {
      var _a;
      if (isElement$1(e.target) && ((_a = this.root.triggerNode) == null ? void 0 : _a.contains(e.target)) && this.root.disableCloseOnTriggerClick) {
        e.preventDefault();
        return;
      }
      this.opts.onInteractOutside.current(e);
      if (e.defaultPrevented) return;
      this.root.handleClose();
    });
    __publicField(this, "onEscapeKeydown", (e) => {
      var _a, _b;
      (_b = (_a = this.opts.onEscapeKeydown).current) == null ? void 0 : _b.call(_a, e);
      if (e.defaultPrevented) return;
      this.root.handleClose();
    });
    __publicField(this, "onOpenAutoFocus", (e) => {
      e.preventDefault();
    });
    __publicField(this, "onCloseAutoFocus", (e) => {
      e.preventDefault();
    });
    __privateAdd(this, _snippetProps2, derived(() => ({ open: this.root.opts.open.current })));
    __privateAdd(this, _props5, derived(() => ({
      id: this.opts.id.current,
      "data-state": this.root.stateAttr,
      "data-disabled": boolToEmptyStrOrUndef(this.root.disabled),
      ...getDataTransitionAttrs(this.root.contentPresence.transitionStatus),
      style: { outline: "none" },
      [tooltipAttrs.content]: "",
      ...this.attachment
    })));
    __publicField(this, "popperProps", {
      onInteractOutside: this.onInteractOutside,
      onEscapeKeydown: this.onEscapeKeydown,
      onOpenAutoFocus: this.onOpenAutoFocus,
      onCloseAutoFocus: this.onCloseAutoFocus
    });
    this.opts = opts;
    this.root = root;
    this.attachment = attachRef(this.opts.ref, (v) => this.root.contentNode = v);
    new SafePolygon({
      triggerNode: () => this.root.triggerNode,
      contentNode: () => this.root.contentNode,
      enabled: () => this.root.opts.open.current && !this.root.disableHoverableContent,
      transitIntentTimeout: 180,
      ignoredTargets: () => {
        if (this.root.provider.opts.skipDelayDuration.current === 0) return [];
        const nodes = [];
        const activeTriggerNode = this.root.triggerNode;
        for (const record of this.root.registry.triggers.values()) {
          if (record.node && record.node !== activeTriggerNode) {
            nodes.push(record.node);
          }
        }
        return nodes;
      },
      onPointerExit: () => {
        if (this.root.provider.isTooltipOpen(this.root)) {
          this.root.handleClose();
        }
      }
    });
  }
  static create(opts) {
    return new _TooltipContentState(opts, TooltipRootContext.get());
  }
  get shouldRender() {
    return this.root.contentPresence.shouldRender;
  }
  get snippetProps() {
    return __privateGet(this, _snippetProps2).call(this);
  }
  set snippetProps($$value) {
    return __privateGet(this, _snippetProps2).call(this, $$value);
  }
  get props() {
    return __privateGet(this, _props5).call(this);
  }
  set props($$value) {
    return __privateGet(this, _props5).call(this, $$value);
  }
};
_snippetProps2 = new WeakMap();
_props5 = new WeakMap();
let TooltipContentState = _TooltipContentState;
function Tooltip($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      open = false,
      triggerId = null,
      onOpenChange = noop,
      onOpenChangeComplete = noop,
      disabled,
      delayDuration,
      disableCloseOnTriggerClick,
      disableHoverableContent,
      ignoreNonKeyboardFocus,
      tether,
      children
    } = $$props;
    const rootState = TooltipRootState.create({
      open: boxWith(() => open, (v) => {
        open = v;
        onOpenChange(v);
      }),
      triggerId: boxWith(() => triggerId, (v) => {
        triggerId = v;
      }),
      delayDuration: boxWith(() => delayDuration),
      disableCloseOnTriggerClick: boxWith(() => disableCloseOnTriggerClick),
      disableHoverableContent: boxWith(() => disableHoverableContent),
      ignoreNonKeyboardFocus: boxWith(() => ignoreNonKeyboardFocus),
      disabled: boxWith(() => disabled),
      onOpenChangeComplete: boxWith(() => onOpenChangeComplete),
      tether: boxWith(() => tether)
    });
    Floating_layer($$renderer2, {
      tooltip: true,
      children: ($$renderer3) => {
        children == null ? void 0 : children($$renderer3, {
          open: rootState.opts.open.current,
          triggerId: rootState.activeTriggerId,
          payload: rootState.activePayload
        });
        $$renderer3.push(`<!---->`);
      }
    });
    bind_props($$props, { open, triggerId });
  });
}
function Tooltip_content($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    const uid = props_id($$renderer2);
    let {
      children,
      child,
      id = createId(uid),
      ref = null,
      side = "top",
      sideOffset = 0,
      align = "center",
      avoidCollisions = true,
      arrowPadding = 0,
      sticky = "partial",
      strategy,
      hideWhenDetached = false,
      customAnchor,
      collisionPadding = 0,
      onInteractOutside = noop,
      onEscapeKeydown = noop,
      forceMount = false,
      style,
      $$slots,
      $$events,
      ...restProps
    } = $$props;
    const contentState = TooltipContentState.create({
      id: boxWith(() => id),
      ref: boxWith(() => ref, (v) => ref = v),
      onInteractOutside: boxWith(() => onInteractOutside),
      onEscapeKeydown: boxWith(() => onEscapeKeydown)
    });
    const floatingProps = derived(() => ({
      side,
      sideOffset,
      align,
      avoidCollisions,
      arrowPadding,
      sticky,
      hideWhenDetached,
      collisionPadding,
      strategy,
      customAnchor: customAnchor ?? contentState.root.triggerNode
    }));
    const mergedProps = derived(() => mergeProps(restProps, floatingProps(), contentState.props));
    if (forceMount) {
      $$renderer2.push("<!--[0-->");
      {
        let popper = function($$renderer3, { props, wrapperProps }) {
          const finalWrapperProps = mergeProps(wrapperProps, {
            style: {
              pointerEvents: contentState.root.disableHoverableContent ? "none" : void 0
            }
          });
          const finalProps = mergeProps(props, { style: getFloatingContentCSSVars("tooltip") }, { style });
          if (child) {
            $$renderer3.push("<!--[0-->");
            child($$renderer3, {
              props: finalProps,
              wrapperProps: finalWrapperProps,
              ...contentState.snippetProps
            });
            $$renderer3.push(`<!---->`);
          } else {
            $$renderer3.push(`<!--[-1--><div${attributes({ ...finalWrapperProps })}><div${attributes({ ...finalProps })}>`);
            children == null ? void 0 : children($$renderer3);
            $$renderer3.push(`<!----></div></div>`);
          }
          $$renderer3.push(`<!--]-->`);
        };
        Popper_layer_force_mount($$renderer2, spread_props([
          mergedProps(),
          contentState.popperProps,
          {
            enabled: contentState.root.opts.open.current,
            id,
            trapFocus: false,
            loop: false,
            preventScroll: false,
            forceMount: true,
            ref: contentState.opts.ref,
            tooltip: true,
            shouldRender: contentState.shouldRender,
            contentPointerEvents: contentState.root.disableHoverableContent ? "none" : "auto",
            popper,
            $$slots: { popper: true }
          }
        ]));
      }
    } else if (!forceMount) {
      $$renderer2.push("<!--[1-->");
      {
        let popper = function($$renderer3, { props, wrapperProps }) {
          const finalWrapperProps = mergeProps(wrapperProps, {
            style: {
              pointerEvents: contentState.root.disableHoverableContent ? "none" : void 0
            }
          });
          const finalProps = mergeProps(props, { style: getFloatingContentCSSVars("tooltip") }, { style });
          if (child) {
            $$renderer3.push("<!--[0-->");
            child($$renderer3, {
              props: finalProps,
              wrapperProps: finalWrapperProps,
              ...contentState.snippetProps
            });
            $$renderer3.push(`<!---->`);
          } else {
            $$renderer3.push(`<!--[-1--><div${attributes({ ...finalWrapperProps })}><div${attributes({ ...finalProps })}>`);
            children == null ? void 0 : children($$renderer3);
            $$renderer3.push(`<!----></div></div>`);
          }
          $$renderer3.push(`<!--]-->`);
        };
        Popper_layer($$renderer2, spread_props([
          mergedProps(),
          contentState.popperProps,
          {
            open: contentState.root.opts.open.current,
            id,
            trapFocus: false,
            loop: false,
            preventScroll: false,
            forceMount: false,
            ref: contentState.opts.ref,
            tooltip: true,
            shouldRender: contentState.shouldRender,
            contentPointerEvents: contentState.root.disableHoverableContent ? "none" : "auto",
            popper,
            $$slots: { popper: true }
          }
        ]));
      }
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]-->`);
    bind_props($$props, { ref });
  });
}
function Tooltip_trigger($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    const uid = props_id($$renderer2);
    let {
      children,
      child,
      id = createId(uid),
      disabled = false,
      payload,
      tether,
      type = "button",
      tabindex = 0,
      ref = null,
      $$slots,
      $$events,
      ...restProps
    } = $$props;
    const triggerState = TooltipTriggerState.create({
      id: boxWith(() => id),
      disabled: boxWith(() => disabled ?? false),
      tabindex: boxWith(() => tabindex ?? 0),
      payload: boxWith(() => payload),
      tether: boxWith(() => tether),
      ref: boxWith(() => ref, (v) => ref = v)
    });
    const mergedProps = derived(() => mergeProps(restProps, triggerState.props, { type }));
    if (child) {
      $$renderer2.push("<!--[0-->");
      child($$renderer2, { props: mergedProps() });
      $$renderer2.push(`<!---->`);
    } else {
      $$renderer2.push(`<!--[-1--><button${attributes({ ...mergedProps() })}>`);
      children == null ? void 0 : children($$renderer2);
      $$renderer2.push(`<!----></button>`);
    }
    $$renderer2.push(`<!--]-->`);
    bind_props($$props, { ref });
  });
}
function Tooltip_provider($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      children,
      delayDuration = 700,
      disableCloseOnTriggerClick = false,
      disableHoverableContent = false,
      disabled = false,
      ignoreNonKeyboardFocus = false,
      skipDelayDuration = 300
    } = $$props;
    TooltipProviderState.create({
      delayDuration: boxWith(() => delayDuration),
      disableCloseOnTriggerClick: boxWith(() => disableCloseOnTriggerClick),
      disableHoverableContent: boxWith(() => disableHoverableContent),
      disabled: boxWith(() => disabled),
      ignoreNonKeyboardFocus: boxWith(() => ignoreNonKeyboardFocus),
      skipDelayDuration: boxWith(() => skipDelayDuration)
    });
    children == null ? void 0 : children($$renderer2);
    $$renderer2.push(`<!---->`);
  });
}
function Tooltip_1($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      text = "",
      side = "top",
      delayDuration = 400,
      class: className = "",
      trigger,
      $$slots,
      $$events,
      ...rest
    } = $$props;
    if (Tooltip_provider) {
      $$renderer2.push("<!--[-->");
      Tooltip_provider($$renderer2, {
        delayDuration,
        disableHoverableContent: true,
        children: ($$renderer3) => {
          if (Tooltip) {
            $$renderer3.push("<!--[-->");
            Tooltip($$renderer3, {
              ignoreNonKeyboardFocus: true,
              children: ($$renderer4) => {
                {
                  let child = function($$renderer5, { props }) {
                    trigger == null ? void 0 : trigger($$renderer5, props);
                    $$renderer5.push(`<!---->`);
                  };
                  if (Tooltip_trigger) {
                    $$renderer4.push("<!--[-->");
                    Tooltip_trigger($$renderer4, { child, $$slots: { child: true } });
                    $$renderer4.push("<!--]-->");
                  } else {
                    $$renderer4.push("<!--[!-->");
                    $$renderer4.push("<!--]-->");
                  }
                }
                $$renderer4.push(` `);
                if (text) {
                  $$renderer4.push("<!--[0-->");
                  if (Portal) {
                    $$renderer4.push("<!--[-->");
                    Portal($$renderer4, {
                      children: ($$renderer5) => {
                        if (Tooltip_content) {
                          $$renderer5.push("<!--[-->");
                          Tooltip_content($$renderer5, spread_props([
                            {
                              side,
                              sideOffset: 6,
                              class: cn("motion-instant z-[70] max-w-[min(18rem,calc(100vw-1rem))] rounded-lg border bg-surface-2 px-2.5 py-1.5 text-caption text-fg-1 shadow-elev-2 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=delayed-open]:animate-in data-[state=delayed-open]:fade-in-0 data-[state=instant-open]:animate-in data-[state=instant-open]:fade-in-0", className)
                            },
                            rest,
                            {
                              children: ($$renderer6) => {
                                $$renderer6.push(`<!---->${escape_html(text)}`);
                              },
                              $$slots: { default: true }
                            }
                          ]));
                          $$renderer5.push("<!--]-->");
                        } else {
                          $$renderer5.push("<!--[!-->");
                          $$renderer5.push("<!--]-->");
                        }
                      }
                    });
                    $$renderer4.push("<!--]-->");
                  } else {
                    $$renderer4.push("<!--[!-->");
                    $$renderer4.push("<!--]-->");
                  }
                } else {
                  $$renderer4.push("<!--[-1-->");
                }
                $$renderer4.push(`<!--]-->`);
              },
              $$slots: { default: true }
            });
            $$renderer3.push("<!--]-->");
          } else {
            $$renderer3.push("<!--[!-->");
            $$renderer3.push("<!--]-->");
          }
        }
      });
      $$renderer2.push("<!--]-->");
    } else {
      $$renderer2.push("<!--[!-->");
      $$renderer2.push("<!--]-->");
    }
  });
}
function Icon_button($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      icon: Icon2,
      label,
      variant = "ghost",
      size: size2 = "icon-sm",
      spin = false,
      tooltip = true,
      side = "top",
      class: className = "",
      $$slots,
      $$events,
      ...rest
    } = $$props;
    function button($$renderer3, extra = {}) {
      Button($$renderer3, spread_props([
        { variant, size: size2, "aria-label": label },
        extra,
        rest,
        {
          class: cn(extra.class, className),
          children: ($$renderer4) => {
            if (Icon2) {
              $$renderer4.push("<!--[0-->");
              if (Icon2) {
                $$renderer4.push("<!--[-->");
                Icon2($$renderer4, { class: spin ? "animate-spin" : "", "aria-hidden": "true" });
                $$renderer4.push("<!--]-->");
              } else {
                $$renderer4.push("<!--[!-->");
                $$renderer4.push("<!--]-->");
              }
            } else {
              $$renderer4.push("<!--[-1-->");
            }
            $$renderer4.push(`<!--]-->`);
          },
          $$slots: { default: true }
        }
      ]));
    }
    if (tooltip) {
      $$renderer2.push("<!--[0-->");
      {
        let trigger = function($$renderer3, props) {
          button($$renderer3, props);
        };
        Tooltip_1($$renderer2, { text: label, side, trigger, $$slots: { trigger: true } });
      }
    } else {
      $$renderer2.push("<!--[-1-->");
      button($$renderer2);
    }
    $$renderer2.push(`<!--]-->`);
  });
}
const chipVariants = tv(
  {
    base: "inline-flex max-w-full min-w-0 items-center gap-1.5 whitespace-nowrap rounded-full border border-transparent px-2.5 py-0.5 text-caption font-medium [&_svg]:size-3 [&_svg]:shrink-0",
    variants: {
      tone: {
        neutral: "bg-muted text-fg-2",
        outline: "border-border bg-transparent text-fg-3",
        working: "bg-status-working/12 text-status-working",
        "needs-you": "bg-status-needs-you/12 text-status-needs-you",
        done: "bg-status-done/12 text-status-done",
        failed: "bg-status-failed/12 text-status-failed",
        paused: "bg-status-paused/12 text-status-paused"
      }
    },
    defaultVariants: { tone: "neutral" }
  },
  { twMergeConfig }
);
const DOT = {
  working: "bg-status-working",
  "needs-you": "bg-status-needs-you",
  done: "bg-status-done",
  failed: "bg-status-failed",
  paused: "bg-status-paused"
};
function Chip($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      tone = "neutral",
      dot = false,
      icon: Icon2 = null,
      class: className = "",
      children,
      $$slots,
      $$events,
      ...rest
    } = $$props;
    $$renderer2.push(`<span${attributes({
      class: clsx(cn(chipVariants({ tone }), className)),
      "data-tone": tone,
      ...rest
    })}>`);
    if (dot && DOT[tone]) {
      $$renderer2.push(`<!--[0--><span${attr_class(clsx(cn("size-1.5 shrink-0 rounded-full", DOT[tone], tone === "working" && "motion-safe:animate-pulse")))} aria-hidden="true"></span>`);
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]-->`);
    if (Icon2) {
      $$renderer2.push("<!--[0-->");
      if (Icon2) {
        $$renderer2.push("<!--[-->");
        Icon2($$renderer2, { "aria-hidden": "true" });
        $$renderer2.push("<!--]-->");
      } else {
        $$renderer2.push("<!--[!-->");
        $$renderer2.push("<!--]-->");
      }
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--><span class="min-w-0 truncate">`);
    children == null ? void 0 : children($$renderer2);
    $$renderer2.push(`<!----></span></span>`);
  });
}
function Popover_1($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      open = false,
      onOpenChange,
      disabled = false,
      sideOffset = 6,
      class: className = "",
      contentClass = "",
      trigger,
      children,
      $$slots,
      $$events,
      ...rest
    } = $$props;
    let $$settled = true;
    let $$inner_renderer;
    function $$render_inner($$renderer3) {
      if (Popover) {
        $$renderer3.push("<!--[-->");
        Popover($$renderer3, {
          onOpenChange,
          get open() {
            return open;
          },
          set open($$value) {
            open = $$value;
            $$settled = false;
          },
          children: ($$renderer4) => {
            if (Popover_trigger) {
              $$renderer4.push("<!--[-->");
              Popover_trigger($$renderer4, spread_props([
                { disabled, class: className },
                rest,
                {
                  children: ($$renderer5) => {
                    trigger == null ? void 0 : trigger($$renderer5);
                    $$renderer5.push(`<!---->`);
                  },
                  $$slots: { default: true }
                }
              ]));
              $$renderer4.push("<!--]-->");
            } else {
              $$renderer4.push("<!--[!-->");
              $$renderer4.push("<!--]-->");
            }
            $$renderer4.push(` `);
            if (Portal) {
              $$renderer4.push("<!--[-->");
              Portal($$renderer4, {
                children: ($$renderer5) => {
                  if (Popover_content) {
                    $$renderer5.push("<!--[-->");
                    Popover_content($$renderer5, {
                      sideOffset,
                      class: cn("motion-standard z-[60] max-w-[calc(100vw-1rem)] overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-xl outline-none data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95", contentClass),
                      children: ($$renderer6) => {
                        children == null ? void 0 : children($$renderer6);
                        $$renderer6.push(`<!---->`);
                      },
                      $$slots: { default: true }
                    });
                    $$renderer5.push("<!--]-->");
                  } else {
                    $$renderer5.push("<!--[!-->");
                    $$renderer5.push("<!--]-->");
                  }
                }
              });
              $$renderer4.push("<!--]-->");
            } else {
              $$renderer4.push("<!--[!-->");
              $$renderer4.push("<!--]-->");
            }
          },
          $$slots: { default: true }
        });
        $$renderer3.push("<!--]-->");
      } else {
        $$renderer3.push("<!--[!-->");
        $$renderer3.push("<!--]-->");
      }
    }
    do {
      $$settled = true;
      $$inner_renderer = $$renderer2.copy();
      $$render_inner($$inner_renderer);
    } while (!$$settled);
    $$renderer2.subsume($$inner_renderer);
    bind_props($$props, { open });
  });
}
function Info_hint($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let { label = "More info", class: className = "", children } = $$props;
    {
      let trigger = function($$renderer3) {
        Info($$renderer3, { class: "size-4", "aria-hidden": "true" });
      };
      Popover_1($$renderer2, {
        "aria-label": label,
        class: cn("motion-standard inline-flex size-6 shrink-0 items-center justify-center rounded-full text-fg-3 outline-none hover:bg-muted hover:text-fg-1 focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:size-11", className),
        contentClass: "w-72 p-3 text-caption text-fg-2",
        trigger,
        children: ($$renderer3) => {
          children == null ? void 0 : children($$renderer3);
          $$renderer3.push(`<!---->`);
        },
        $$slots: { trigger: true, default: true }
      });
    }
  });
}
const SHELL_KEY = Symbol("el-shell");
function Page_header($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      title,
      icon: Icon2 = null,
      scope = "",
      scopeIcon = null,
      hint = "",
      size: size2 = "page",
      onBack = null,
      backLabel = "Back",
      onMenu = void 0,
      menuLabel = void 0,
      subtitle = "",
      titleAttrs = {},
      scopeAttrs = {},
      backOn = "always",
      meta = null,
      actions = null,
      class: className = "",
      $$slots,
      $$events,
      ...rest
    } = $$props;
    const shell = getContext(SHELL_KEY);
    const phoneBack = derived(() => Boolean(onBack) && backOn !== "desktop");
    const openMenu = derived(() => phoneBack() ? null : onMenu === void 0 ? shell == null ? void 0 : shell.openMenu : onMenu);
    const menuName = derived(() => menuLabel ?? (shell == null ? void 0 : shell.menuLabel) ?? "Menu");
    $$renderer2.push(`<header${attributes({
      class: clsx(cn("flex min-h-11 min-w-0 items-center gap-2 md:gap-3", className)),
      "data-page-header": true,
      ...rest
    })}>`);
    if (openMenu()) {
      $$renderer2.push("<!--[0-->");
      Icon_button($$renderer2, {
        icon: Menu,
        label: menuName(),
        size: "icon-sm",
        class: "-ml-2 md:hidden",
        onclick: openMenu()
      });
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--> `);
    if (onBack) {
      $$renderer2.push("<!--[0-->");
      Icon_button($$renderer2, {
        icon: Arrow_left,
        label: backLabel,
        size: "icon-sm",
        class: cn("-ml-2", backOn === "phone" && "md:hidden", backOn === "desktop" && "max-md:hidden"),
        onclick: onBack
      });
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--> `);
    if (Icon2 && size2 === "page") {
      $$renderer2.push(`<!--[0--><span class="grid size-10 shrink-0 place-items-center rounded-xl bg-card text-fg-2 shadow-elev-1 max-md:hidden" aria-hidden="true">`);
      if (Icon2) {
        $$renderer2.push("<!--[-->");
        Icon2($$renderer2, { class: "size-5" });
        $$renderer2.push("<!--]-->");
      } else {
        $$renderer2.push("<!--[!-->");
        $$renderer2.push("<!--]-->");
      }
      $$renderer2.push(`</span>`);
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--> <div${attributes({ class: "flex min-w-0 flex-col", ...titleAttrs })}><h1${attr_class(clsx(cn("min-w-0 truncate text-fg-1", size2 === "compact" ? "text-title" : "text-headline max-md:text-title")))}>${escape_html(title)}</h1> `);
    if (subtitle) {
      $$renderer2.push(`<!--[0--><span class="truncate text-caption text-fg-3 max-sm:hidden">${escape_html(subtitle)}</span>`);
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--></div> `);
    if (scope) {
      $$renderer2.push("<!--[0-->");
      Chip($$renderer2, spread_props([
        {
          tone: "outline",
          icon: scopeIcon,
          class: "shrink-[2] max-sm:max-w-[45%]",
          "data-scope-chip": true
        },
        scopeAttrs,
        {
          children: ($$renderer3) => {
            $$renderer3.push(`<!---->${escape_html(scope)}`);
          },
          $$slots: { default: true }
        }
      ]));
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--> `);
    meta == null ? void 0 : meta($$renderer2);
    $$renderer2.push(`<!----> `);
    if (hint) {
      $$renderer2.push("<!--[0-->");
      Info_hint($$renderer2, {
        label: `About ${title.toLowerCase()}`,
        children: ($$renderer3) => {
          if (typeof hint === "string") {
            $$renderer3.push(`<!--[0-->${escape_html(hint)}`);
          } else {
            $$renderer3.push("<!--[-1-->");
            hint($$renderer3);
            $$renderer3.push(`<!---->`);
          }
          $$renderer3.push(`<!--]-->`);
        }
      });
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--> `);
    if (actions) {
      $$renderer2.push(`<!--[0--><div class="ml-auto flex shrink-0 items-center gap-1">`);
      actions($$renderer2);
      $$renderer2.push(`<!----></div>`);
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--></header>`);
  });
}
function Section($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      title,
      description = "",
      icon: Icon2 = null,
      hint = null,
      actions = null,
      collapsible = false,
      open = true,
      headingLevel = 2,
      class: className = "",
      children,
      $$slots,
      $$events,
      ...rest
    } = $$props;
    const heading = derived(() => `h${Math.min(6, Math.max(1, headingLevel))}`);
    function head($$renderer3) {
      if (Icon2) {
        $$renderer3.push("<!--[0-->");
        if (Icon2) {
          $$renderer3.push("<!--[-->");
          Icon2($$renderer3, { class: "size-4 shrink-0 text-fg-3", "aria-hidden": "true" });
          $$renderer3.push("<!--]-->");
        } else {
          $$renderer3.push("<!--[!-->");
          $$renderer3.push("<!--]-->");
        }
      } else {
        $$renderer3.push("<!--[-1-->");
      }
      $$renderer3.push(`<!--]--> `);
      element(
        $$renderer3,
        heading(),
        () => {
          $$renderer3.push(` class="min-w-0 truncate text-title text-fg-1"`);
        },
        () => {
          $$renderer3.push(`${escape_html(title)}`);
        }
      );
      $$renderer3.push(` `);
      if (hint) {
        $$renderer3.push(`<!--[0--><span class="contents" role="presentation">`);
        Info_hint($$renderer3, {
          label: `About ${title}`,
          children: ($$renderer4) => {
            if (typeof hint === "string") {
              $$renderer4.push(`<!--[0-->${escape_html(hint)}`);
            } else {
              $$renderer4.push("<!--[-1-->");
              hint($$renderer4);
              $$renderer4.push(`<!---->`);
            }
            $$renderer4.push(`<!--]-->`);
          }
        });
        $$renderer3.push(`<!----></span>`);
      } else {
        $$renderer3.push("<!--[-1-->");
      }
      $$renderer3.push(`<!--]-->`);
    }
    if (collapsible) {
      $$renderer2.push(`<!--[0--><details${attributes({
        class: clsx(cn("group flex flex-col gap-3", className)),
        open,
        "data-section": true,
        ...rest
      })}><summary class="flex min-h-9 cursor-pointer list-none items-center gap-2 rounded-lg outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:min-h-11 [&amp;::-webkit-details-marker]:hidden">`);
      head($$renderer2);
      $$renderer2.push(`<!----> `);
      Chevron_down($$renderer2, {
        class: "motion-standard ml-auto size-4 shrink-0 text-fg-3 group-open:rotate-180",
        "aria-hidden": "true"
      });
      $$renderer2.push(`<!----></summary> `);
      if (description) {
        $$renderer2.push(`<!--[0--><p class="-mt-1 text-caption text-fg-3">${escape_html(description)}</p>`);
      } else {
        $$renderer2.push("<!--[-1-->");
      }
      $$renderer2.push(`<!--]--> `);
      children == null ? void 0 : children($$renderer2);
      $$renderer2.push(`<!----></details>`);
    } else {
      $$renderer2.push(`<!--[-1--><section${attributes({
        class: clsx(cn("flex flex-col gap-3", className)),
        "data-section": true,
        ...rest
      })}><header class="flex min-h-9 items-center gap-2 max-md:min-h-11">`);
      head($$renderer2);
      $$renderer2.push(`<!----> `);
      if (actions) {
        $$renderer2.push(`<!--[0--><div class="ml-auto flex shrink-0 items-center gap-1">`);
        actions($$renderer2);
        $$renderer2.push(`<!----></div>`);
      } else {
        $$renderer2.push("<!--[-1-->");
      }
      $$renderer2.push(`<!--]--></header> `);
      if (description) {
        $$renderer2.push(`<!--[0--><p class="-mt-2 text-caption text-fg-3">${escape_html(description)}</p>`);
      } else {
        $$renderer2.push("<!--[-1-->");
      }
      $$renderer2.push(`<!--]--> `);
      children == null ? void 0 : children($$renderer2);
      $$renderer2.push(`<!----></section>`);
    }
    $$renderer2.push(`<!--]-->`);
    bind_props($$props, { open });
  });
}
const icons = Object.freeze({
  session: Message_square,
  run: Layers,
  ticket: Ticket,
  subagent: Bot,
  steer: Signpost,
  followUp: List_end,
  report: File_text,
  memory: Brain,
  quota: Gauge,
  provider: Plug,
  project: Folder_git_2,
  git: Git_branch,
  compress: Shrink,
  copy: Copy,
  share: Share_2,
  edit: Pencil,
  retry: Rotate_ccw,
  dismiss: X,
  // Cancel a queued message: the row's own state glyph, not Dismiss.
  cancel: Ban,
  // Re-reading live data (quota, status). Retry is re-running a failed action.
  refresh: Refresh_cw,
  // The model a Session or action runs on.
  model: Cpu
});
const statusIcons = Object.freeze({
  working: Loader,
  "needs-you": Circle_alert,
  done: Circle_check,
  failed: Circle_x,
  paused: Circle_pause
});
const statusLabels = Object.freeze({
  working: "Working",
  "needs-you": "Needs you",
  done: "Done",
  failed: "Failed",
  paused: "Paused"
});
function Row($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    const GLYPH_TONE = {
      working: "text-status-working",
      "needs-you": "text-status-needs-you",
      done: "text-status-done",
      failed: "text-status-failed",
      paused: "text-status-paused",
      idle: "text-fg-4"
    };
    let {
      title,
      meta = "",
      icon: Icon2 = null,
      status = null,
      unread = false,
      statusWord = true,
      attention = false,
      current = false,
      href = null,
      onclick = null,
      selected = false,
      badges = null,
      trailing = null,
      class: className = "",
      $$slots,
      $$events,
      ...rest
    } = $$props;
    const interactive = derived(() => Boolean(href || onclick));
    const Glyph = derived(() => status === "idle" ? Circle : status && DOT[status] ? statusIcons[status] : null);
    const cls = derived(() => cn("flex min-h-12 w-full min-w-0 items-center gap-3 rounded-xl px-3 py-2 text-left max-md:min-h-14", interactive() && "motion-standard cursor-pointer outline-none hover:bg-surface-3/60 focus-visible:ring-[3px] focus-visible:ring-ring/40", (selected || current) && "bg-surface-3", attention && "border border-status-needs-you/30 bg-status-needs-you/10 hover:bg-status-needs-you/15", className));
    function body($$renderer3) {
      if (Glyph()) {
        $$renderer3.push(`<!--[0--><span${attr_class(clsx(cn("grid size-5 shrink-0 place-items-center", GLYPH_TONE[status])))}${attr("data-status", status)} aria-hidden="true">`);
        if (Glyph()) {
          $$renderer3.push("<!--[-->");
          Glyph()($$renderer3, {
            class: cn("size-4", status === "working" && "motion-safe:animate-pulse")
          });
          $$renderer3.push("<!--]-->");
        } else {
          $$renderer3.push("<!--[!-->");
          $$renderer3.push("<!--]-->");
        }
        $$renderer3.push(`</span> `);
        if (statusWord) {
          $$renderer3.push(`<!--[0--><span class="sr-only">${escape_html(status === "idle" ? "Idle" : statusLabels[status])}.</span>`);
        } else {
          $$renderer3.push("<!--[-1-->");
        }
        $$renderer3.push(`<!--]-->`);
      } else {
        $$renderer3.push("<!--[-1-->");
      }
      $$renderer3.push(`<!--]--> `);
      if (Icon2) {
        $$renderer3.push(`<!--[0--><span class="grid size-8 shrink-0 place-items-center rounded-lg bg-muted text-fg-2" aria-hidden="true">`);
        if (Icon2) {
          $$renderer3.push("<!--[-->");
          Icon2($$renderer3, { class: "size-4" });
          $$renderer3.push("<!--]-->");
        } else {
          $$renderer3.push("<!--[!-->");
          $$renderer3.push("<!--]-->");
        }
        $$renderer3.push(`</span>`);
      } else {
        $$renderer3.push("<!--[-1-->");
      }
      $$renderer3.push(`<!--]--> <span class="flex min-w-0 flex-1 flex-col"><span class="flex min-w-0 items-center gap-1.5"><span class="min-w-0 truncate text-body text-fg-1">${escape_html(title)}</span> `);
      if (badges) {
        $$renderer3.push("<!--[0-->");
        badges($$renderer3);
        $$renderer3.push(`<!---->`);
      } else {
        $$renderer3.push("<!--[-1-->");
      }
      $$renderer3.push(`<!--]--></span> `);
      if (meta) {
        $$renderer3.push(`<!--[0--><span class="truncate text-caption text-fg-3">${escape_html(meta)}</span>`);
      } else {
        $$renderer3.push("<!--[-1-->");
      }
      $$renderer3.push(`<!--]--></span> `);
      if (unread) {
        $$renderer3.push(`<!--[0--><span class="size-2 shrink-0 rounded-full bg-status-working" data-unread="" aria-hidden="true"></span> <span class="sr-only">Unread.</span>`);
      } else {
        $$renderer3.push("<!--[-1-->");
      }
      $$renderer3.push(`<!--]--> `);
      if (trailing) {
        $$renderer3.push(`<!--[0--><span class="flex shrink-0 items-center gap-2">`);
        trailing($$renderer3);
        $$renderer3.push(`<!----></span>`);
      } else {
        $$renderer3.push("<!--[-1-->");
      }
      $$renderer3.push(`<!--]-->`);
    }
    if (href) {
      $$renderer2.push(`<!--[0--><a${attributes({
        href,
        class: clsx(cls()),
        "aria-current": current ? "page" : selected ? "true" : void 0,
        ...rest
      })}>`);
      body($$renderer2);
      $$renderer2.push(`<!----></a>`);
    } else if (onclick) {
      $$renderer2.push(`<!--[1--><button${attributes({
        type: "button",
        class: clsx(cls()),
        "aria-current": current ? "true" : void 0,
        "aria-pressed": !current && selected ? "true" : void 0,
        ...rest
      })}>`);
      body($$renderer2);
      $$renderer2.push(`<!----></button>`);
    } else {
      $$renderer2.push(`<!--[-1--><div${attributes({ class: clsx(cls()), ...rest })}>`);
      body($$renderer2);
      $$renderer2.push(`<!----></div>`);
    }
    $$renderer2.push(`<!--]-->`);
  });
}
function Stat_card($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    const TONE = {
      working: "text-status-working",
      "needs-you": "text-status-needs-you",
      done: "text-status-done",
      failed: "text-status-failed",
      paused: "text-status-paused"
    };
    let {
      label,
      value,
      unit = "",
      caption = "",
      tone = null,
      icon: Icon2 = null,
      class: className = "",
      $$slots,
      $$events,
      ...rest
    } = $$props;
    $$renderer2.push(`<div${attributes({
      class: clsx(cn("flex min-w-0 flex-col gap-1.5 rounded-2xl border bg-card p-4 shadow-elev-1 max-sm:p-3", className)),
      "data-stat-card": true,
      ...rest
    })}><div class="flex min-w-0 items-center gap-1.5 text-caption text-fg-3">`);
    if (Icon2) {
      $$renderer2.push("<!--[0-->");
      if (Icon2) {
        $$renderer2.push("<!--[-->");
        Icon2($$renderer2, { class: "size-3.5 shrink-0", "aria-hidden": "true" });
        $$renderer2.push("<!--]-->");
      } else {
        $$renderer2.push("<!--[!-->");
        $$renderer2.push("<!--]-->");
      }
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--> <span class="truncate">${escape_html(label)}</span></div> <div${attr_class(clsx(cn("flex flex-wrap items-baseline gap-x-0.5", tone ? TONE[tone] : "text-fg-1")))}><span class="text-numeric tabular-nums max-sm:text-[1.5rem]" data-stat-value="">${escape_html(value)}</span> `);
    if (unit) {
      $$renderer2.push(`<!--[0--><span class="text-title tabular-nums opacity-70">${escape_html(unit)}</span>`);
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--></div> `);
    if (caption) {
      $$renderer2.push(`<!--[0--><div class="truncate text-caption text-fg-3">${escape_html(caption)}</div>`);
    } else {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--></div>`);
  });
}
function Empty_state($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let {
      icon: Icon2 = Inbox,
      title,
      description = "",
      action = null,
      compact = false,
      class: className = "",
      $$slots,
      $$events,
      ...rest
    } = $$props;
    if (compact) {
      $$renderer2.push(`<!--[0--><div${attributes({
        class: clsx(cn("flex flex-wrap items-center gap-2.5 rounded-xl border border-dashed px-3 py-2.5 text-left", className)),
        "data-empty-state": true,
        "data-compact": true,
        ...rest
      })}><span class="grid size-7 shrink-0 place-items-center rounded-full bg-muted text-fg-3" aria-hidden="true">`);
      if (Icon2) {
        $$renderer2.push("<!--[-->");
        Icon2($$renderer2, { class: "size-3.5" });
        $$renderer2.push("<!--]-->");
      } else {
        $$renderer2.push("<!--[!-->");
        $$renderer2.push("<!--]-->");
      }
      $$renderer2.push(`</span> <div class="flex min-w-0 flex-1 flex-col"><p class="text-body text-fg-1">${escape_html(title)}</p> `);
      if (description) {
        $$renderer2.push(`<!--[0--><p class="text-caption text-fg-3">${escape_html(description)}</p>`);
      } else {
        $$renderer2.push("<!--[-1-->");
      }
      $$renderer2.push(`<!--]--></div> `);
      if (action) {
        $$renderer2.push(`<!--[0--><div class="shrink-0">`);
        action($$renderer2);
        $$renderer2.push(`<!----></div>`);
      } else {
        $$renderer2.push("<!--[-1-->");
      }
      $$renderer2.push(`<!--]--></div>`);
    } else {
      $$renderer2.push(`<!--[-1--><div${attributes({
        class: clsx(cn("flex flex-col items-center gap-3 rounded-2xl border border-dashed px-6 py-10 text-center", className)),
        "data-empty-state": true,
        ...rest
      })}><span class="grid size-11 place-items-center rounded-full bg-muted text-fg-3" aria-hidden="true">`);
      if (Icon2) {
        $$renderer2.push("<!--[-->");
        Icon2($$renderer2, { class: "size-5" });
        $$renderer2.push("<!--]-->");
      } else {
        $$renderer2.push("<!--[!-->");
        $$renderer2.push("<!--]-->");
      }
      $$renderer2.push(`</span> <div class="flex flex-col gap-1"><p class="text-title text-fg-1">${escape_html(title)}</p> `);
      if (description) {
        $$renderer2.push(`<!--[0--><p class="text-caption text-fg-3">${escape_html(description)}</p>`);
      } else {
        $$renderer2.push("<!--[-1-->");
      }
      $$renderer2.push(`<!--]--></div> `);
      if (action) {
        $$renderer2.push(`<!--[0--><div class="mt-1">`);
        action($$renderer2);
        $$renderer2.push(`<!----></div>`);
      } else {
        $$renderer2.push("<!--[-1-->");
      }
      $$renderer2.push(`<!--]--></div>`);
    }
    $$renderer2.push(`<!--]-->`);
  });
}
function Skeleton($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let { class: className = "", $$slots, $$events, ...rest } = $$props;
    $$renderer2.push(`<div${attributes({
      "aria-hidden": "true",
      class: clsx(cn("h-4 rounded-md bg-surface-3/70 motion-safe:animate-pulse", className)),
      ...rest
    })}></div>`);
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
      runs: (group.runs ?? []).filter((run2) => run2.status === "running" || run2.status === "needs-you")
    })).filter((group) => group.sessions.length || group.runs.length).sort((a, b) => a.name.localeCompare(b.name)));
    const specLabel = (run2) => String(run2.spec ?? run2.id).split("/").filter(Boolean).at(-2) || run2.spec || run2.id;
    const entries = derived(() => activeGroups().flatMap((group) => [
      ...group.sessions.map((session) => ({
        kind: "session",
        testid: "dashboard-session",
        key: `s:${session.id}`,
        title: session.title || session.id,
        needs: session.status === "needs-you",
        meta: ["Session", group.name, session.harness].filter(Boolean).join(" · "),
        path: buildPath({ name: "session", id: session.id })
      })),
      ...group.runs.map((run2) => {
        var _a2;
        return {
          kind: "run",
          testid: "dashboard-run",
          key: `r:${run2.id}`,
          title: specLabel(run2),
          needs: run2.status === "needs-you",
          meta: [
            "Run",
            group.name,
            ((_a2 = run2.counts) == null ? void 0 : _a2.total) ? `${run2.counts.merged ?? 0}/${run2.counts.total} merged` : ""
          ].filter(Boolean).join(" · "),
          path: buildPath({
            name: "project-run",
            projectId: group.projectId,
            runId: run2.id
          })
        };
      })
    ]));
    const needsYou = derived(() => entries().filter((entry) => entry.needs));
    const running = derived(() => entries().filter((entry) => !entry.needs));
    const projectRows = derived(() => activeGroups().map((group) => {
      const items = [
        ...group.sessions.map((s) => s.status === "needs-you"),
        ...group.runs.map((r2) => r2.status === "needs-you")
      ];
      const needs = items.filter(Boolean).length;
      const parts = [
        needs ? `${needs} needs you` : "",
        items.length - needs ? `${items.length - needs} running` : ""
      ].filter(Boolean);
      return { group, needs, meta: parts.join(" · ") };
    }));
    $$renderer2.push(`<div class="min-h-0 flex-1 overflow-auto" data-testid="dashboard"><div class="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 pt-4 pb-[max(4rem,env(safe-area-inset-bottom))] md:px-6 md:pt-6">`);
    Page_header($$renderer2, {
      title: "Dashboard",
      icon: Layout_dashboard,
      scope: "All projects",
      scopeIcon: icons.project,
      class: "animate-rise"
    });
    $$renderer2.push(`<!----> `);
    {
      $$renderer2.push("<!--[-1-->");
    }
    $$renderer2.push(`<!--]--> `);
    if (!loaded) {
      $$renderer2.push(`<!--[0--><div role="status" aria-label="Loading Dashboard" class="flex flex-col gap-6"><div class="grid grid-cols-2 gap-3 md:gap-4"><!--[-->`);
      const each_array = ensure_array_like([0, 1]);
      for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
        each_array[$$index];
        Skeleton($$renderer2, { class: "h-24 rounded-2xl" });
      }
      $$renderer2.push(`<!--]--></div> <div class="flex flex-col gap-2"><!--[-->`);
      const each_array_1 = ensure_array_like([0, 1, 2]);
      for (let $$index_1 = 0, $$length = each_array_1.length; $$index_1 < $$length; $$index_1++) {
        each_array_1[$$index_1];
        Skeleton($$renderer2, { class: "h-14 rounded-xl" });
      }
      $$renderer2.push(`<!--]--></div></div>`);
    } else {
      $$renderer2.push(`<!--[1--><div class="animate-rise grid grid-cols-2 gap-3 md:gap-4" data-testid="dashboard-stats">`);
      Stat_card($$renderer2, {
        label: "Needs you",
        value: needsYou().length,
        tone: needsYou().length ? "needs-you" : null,
        icon: statusIcons["needs-you"],
        caption: needsYou().length ? "Open the first one" : "All clear"
      });
      $$renderer2.push(`<!----> `);
      Stat_card($$renderer2, {
        label: "Running",
        value: running().length,
        tone: running().length ? "working" : null,
        icon: statusIcons.working,
        caption: running().length ? "Working now" : "Nothing running"
      });
      $$renderer2.push(`<!----></div> `);
      if (entries().length) {
        $$renderer2.push("<!--[0-->");
        if (needsYou().length) {
          $$renderer2.push("<!--[0-->");
          Section($$renderer2, {
            title: "Needs you",
            icon: statusIcons["needs-you"],
            class: "animate-rise",
            "data-testid": "dashboard-needs-you",
            children: ($$renderer3) => {
              $$renderer3.push(`<ul class="flex flex-col gap-1"><!--[-->`);
              const each_array_2 = ensure_array_like(needsYou());
              for (let $$index_2 = 0, $$length = each_array_2.length; $$index_2 < $$length; $$index_2++) {
                let entry = each_array_2[$$index_2];
                $$renderer3.push(`<li>`);
                Row($$renderer3, {
                  title: entry.title,
                  meta: entry.meta,
                  status: "needs-you",
                  attention: true,
                  onclick: () => onNavigate(entry.path),
                  "data-testid": entry.testid
                });
                $$renderer3.push(`<!----></li>`);
              }
              $$renderer3.push(`<!--]--></ul>`);
            },
            $$slots: { default: true }
          });
        } else {
          $$renderer2.push("<!--[-1-->");
        }
        $$renderer2.push(`<!--]--> `);
        if (running().length) {
          $$renderer2.push("<!--[0-->");
          Section($$renderer2, {
            title: "Running",
            icon: statusIcons.working,
            class: "animate-rise",
            "data-testid": "dashboard-running",
            children: ($$renderer3) => {
              $$renderer3.push(`<ul class="flex flex-col gap-1"><!--[-->`);
              const each_array_3 = ensure_array_like(running());
              for (let $$index_3 = 0, $$length = each_array_3.length; $$index_3 < $$length; $$index_3++) {
                let entry = each_array_3[$$index_3];
                $$renderer3.push(`<li>`);
                Row($$renderer3, {
                  title: entry.title,
                  meta: entry.meta,
                  status: "working",
                  onclick: () => onNavigate(entry.path),
                  "data-testid": entry.testid
                });
                $$renderer3.push(`<!----></li>`);
              }
              $$renderer3.push(`<!--]--></ul>`);
            },
            $$slots: { default: true }
          });
        } else {
          $$renderer2.push("<!--[-1-->");
        }
        $$renderer2.push(`<!--]--> `);
        Section($$renderer2, {
          title: "Projects",
          icon: icons.project,
          description: "Open a Project to see all its Sessions and Runs.",
          class: "animate-rise",
          children: ($$renderer3) => {
            $$renderer3.push(`<ul class="flex flex-col gap-1"><!--[-->`);
            const each_array_4 = ensure_array_like(projectRows());
            for (let $$index_4 = 0, $$length = each_array_4.length; $$index_4 < $$length; $$index_4++) {
              let { group, needs, meta } = each_array_4[$$index_4];
              $$renderer3.push(`<li>`);
              {
                let trailing = function($$renderer4) {
                  Chevron_right($$renderer4, { class: "size-4 text-fg-3", "aria-hidden": "true" });
                };
                Row($$renderer3, {
                  title: group.name,
                  meta,
                  status: needs ? "needs-you" : "working",
                  onclick: () => onNavigate(buildPath({ name: "project", projectId: group.projectId })),
                  "data-testid": "dashboard-project",
                  "aria-label": `Open ${stringify(group.name)}`,
                  trailing,
                  $$slots: { trailing: true }
                });
              }
              $$renderer3.push(`<!----></li>`);
            }
            $$renderer3.push(`<!--]--></ul>`);
          },
          $$slots: { default: true }
        });
        $$renderer2.push(`<!---->`);
      } else {
        $$renderer2.push("<!--[-1-->");
        Empty_state($$renderer2, {
          icon: statusIcons.done,
          title: "Nothing needs attention",
          description: "Running and needs-you work appears here.",
          class: "animate-rise"
        });
      }
      $$renderer2.push(`<!--]-->`);
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
