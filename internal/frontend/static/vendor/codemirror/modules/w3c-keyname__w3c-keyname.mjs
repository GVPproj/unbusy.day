/*!
Copyright (C) 2016 by Marijn Haverbeke <marijn@haverbeke.berlin> and others

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
/* esm.sh - w3c-keyname@2.2.8 */
var t={8:"Backspace",9:"Tab",10:"Enter",12:"NumLock",13:"Enter",16:"Shift",17:"Control",18:"Alt",20:"CapsLock",27:"Escape",32:" ",33:"PageUp",34:"PageDown",35:"End",36:"Home",37:"ArrowLeft",38:"ArrowUp",39:"ArrowRight",40:"ArrowDown",44:"PrintScreen",45:"Insert",46:"Delete",59:";",61:"=",91:"Meta",92:"Meta",106:"*",107:"+",108:",",109:"-",110:".",111:"/",144:"NumLock",145:"ScrollLock",160:"Shift",161:"Shift",162:"Control",163:"Control",164:"Alt",165:"Alt",173:"-",186:";",187:"=",188:",",189:"-",190:".",191:"/",192:"`",219:"[",220:"\\",221:"]",222:"'"},i={48:")",49:"!",50:"@",51:"#",52:"$",53:"%",54:"^",55:"&",56:"*",57:"(",59:":",61:"+",173:"_",186:":",187:"+",188:"<",189:"_",190:">",191:"?",192:"~",219:"{",220:"|",221:"}",222:'"'},n=typeof navigator<"u"&&/Mac/.test(navigator.platform),y=typeof navigator<"u"&&/MSIE \d|Trident\/(?:[7-9]|\d{2,})\..*rv:(\d+)/.exec(navigator.userAgent);for(r=0;r<10;r++)t[48+r]=t[96+r]=String(r);var r;for(r=1;r<=24;r++)t[r+111]="F"+r;var r;for(r=65;r<=90;r++)t[r]=String.fromCharCode(r+32),i[r]=String.fromCharCode(r);var r;for(a in t)i.hasOwnProperty(a)||(i[a]=t[a]);var a;function g(o){var f=n&&o.metaKey&&o.shiftKey&&!o.ctrlKey&&!o.altKey||y&&o.shiftKey&&o.key&&o.key.length==1||o.key=="Unidentified",e=!f&&o.key||(o.shiftKey?i:t)[o.keyCode]||o.key||"Unidentified";return e=="Esc"&&(e="Escape"),e=="Del"&&(e="Delete"),e=="Left"&&(e="ArrowLeft"),e=="Up"&&(e="ArrowUp"),e=="Right"&&(e="ArrowRight"),e=="Down"&&(e="ArrowDown"),e}export{t as base,g as keyName,i as shift};
