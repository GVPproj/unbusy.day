/*!
Copyright (C) 2020 by Marijn Haverbeke <marijn@haverbeke.berlin>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
/* esm.sh - crelt@1.0.7 */
function s(){var r=arguments[0];typeof r=="string"&&(r=document.createElement(r));var e=1,t=arguments[1];if(t&&typeof t=="object"&&t.nodeType==null&&!Array.isArray(t)){for(var n in t)if(Object.prototype.hasOwnProperty.call(t,n)){var o=t[n];typeof o=="string"?r.setAttribute(n,o):o!=null&&(r[n]=o)}e++}for(;e<arguments.length;e++)f(r,arguments[e]);return r}function f(r,e){if(typeof e=="string")r.appendChild(document.createTextNode(e));else if(e!=null)if(e.nodeType!=null)r.appendChild(e);else if(Array.isArray(e))for(var t=0;t<e.length;t++)f(r,e[t]);else throw new RangeError("Unsupported child node: "+e)}export{s as default};
