# cose-wg/Examples vectors

`vectors.txt` lists the `output.cbor` value of every example in
[cose-wg/Examples](https://github.com/cose-wg/Examples) at commit
`53c9d634333bb4f529d78f5980fffa2667ee2c12`, one per line as
`path hex`, followed by `fail` for examples the corpus marks as
intentionally invalid at the COSE level. The examples are released into the public domain by their
authors (see that repository's LICENSE).

Regenerate with:

```sh
git clone https://github.com/cose-wg/Examples && cd Examples
python3 -I -c 'import json,glob
for p in sorted(glob.glob("**/*.json", recursive=True)):
    d = json.load(open(p, encoding="utf-8-sig"))
    o = d.get("output", {})
    if isinstance(o, dict) and o.get("cbor"):
        print(p, o["cbor"].lower(), *(["fail"] if d.get("fail") else []))'
```
