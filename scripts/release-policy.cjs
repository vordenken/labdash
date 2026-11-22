'use strict';

function newerStableTag(version, tags) {
  const parts = version.split('.').map(BigInt);
  for (const tag of tags) {
    if (!/^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(tag)) continue;
    const other = tag.slice(1).split('.').map(BigInt);
    const index = other.findIndex((value, i) => value !== parts[i]);
    if (index !== -1 && other[index] > parts[index]) return tag;
  }
  return null;
}

module.exports = { newerStableTag };
