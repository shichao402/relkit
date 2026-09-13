# relkit

RUP锛圧elease & Update Protocol锛夌殑 Go 瀹炵幇浠撳簱锛氬彂甯?CLI + 鑷墭绠″垎鍙戞湇鍔★紝鍚屼竴妯″潡銆佷袱濂楀崟浜岃繘鍒躲€?

| 浜岃繘鍒?| 璺緞 | 浣滅敤 |
|---|---|---|
| `relkit` | `cmd/relkit` | stage / 绛惧悕 / 涓婁紶 / 鎻愪氦 |
| `relkit-agent` | `cmd/relkit-agent` | CI 浜?staged 鏍戯紱鏈満鎸侀挜鍐欏叆鏁版嵁闈?|
| `relkit-serve` | `cmd/relkit-serve` | Range 涓嬭浇 + CAS 鑳藉姏涓婁紶 + 瀛ゅ効 GC |

褰撳墠鐗堟湰锛歚0.3.14`锛圧UP **protobuf v2** 绾挎牸寮忥級

鏇剧敤杩囧叾瀹冭瑷€鍋氳繃鍘熷瀷锛涘彂甯冨伐鍏锋寮忓疄鐜板氨鏄湰浠撳簱鐨?Go CLI銆傝 [`docs/adr/0001-go-only-publisher.md`](docs/adr/0001-go-only-publisher.md)銆? 
CLI 涓?serve 鍚堝苟鍐崇瓥瑙?[`docs/adr/0002-one-repo-cli-and-serve.md`](docs/adr/0002-one-repo-cli-and-serve.md)銆? 
Protobuf 绾挎牸寮忚 [`docs/adr/0003-protobuf-v2-wire-format.md`](docs/adr/0003-protobuf-v2-wire-format.md)锛涚粨鏋?SSOT 鍦ㄦ湰浠?[`proto/`](proto/)銆? 
椤圭洰鐗堟湰 SSOT 瑙?[`docs/adr/0004-project-version-ssot.md`](docs/adr/0004-project-version-ssot.md)锛歚VERSION.json` + `relkit version 鈥銆? 
鎿嶄綔闈㈡澘涓€娆℃€у紩瀵煎嚟鎹 [`docs/adr/0006-admin-panel-bootstrap.md`](docs/adr/0006-admin-panel-bootstrap.md)銆?

## 浜у搧寮€绠?

缁欏彟涓€涓骇鍝佷粨鎺ュ叆鏃讹紝澶嶅埗 `scripts/host/`锛岃窇 `python scripts/host/relkit_host.py`銆係kill锛歔`skills/relkit-ops/SKILL.md`](skills/relkit-ops/SKILL.md)銆傜┖鏈鸿 systemd / 鎹簩杩涘埗鎵嶇敤鏈粨 [`deploy/README.md`](deploy/README.md)銆?

## 瀹夎

涓讳粨鏄?[github.com/shichao402/relkit](https://github.com/shichao402/relkit)銆侴o 妯″潡鍚嶆槸閫昏緫璺緞 `go.firoyang.com/relkit`锛?*娌℃湁** vanity 瑙ｆ瀽锛屼笉瑕?`go get` / `go install` 璇ユā鍧椼€?

鏈粨搴撳紑鍙戣€呬粠 [Releases](https://github.com/shichao402/relkit/releases) 鍙栦簩杩涘埗锛屾垨鍦ㄦ湰浠撶敤 `python deploy/relkit.py build`銆?

瀹夸富浜у搧浠撲娇鐢?`relkit.consume/2` lock 閽変綇 Release銆乧ommit銆侀檮浠?URL銆丼HA-256 浠ュ強 `scripts/host/` 鏍戝搱甯岋紝鍐嶇敱 `python scripts/host/relkit_host.py install` 瀹夎 SDK銆丆LI 涓?updater銆傛秷璐圭涓?clone 鏈粨銆佷笉瀹夎 Go锛屼篃涓嶄粠婧愮爜鏋勫缓銆?
lock 绀轰緥瑙?[`scripts/relkit.lock.example.json`](scripts/relkit.lock.example.json)銆?

## relkit锛堝彂甯?CLI锛?

```text
relkit init
relkit keygen
relkit version   # get|set|bump|code|path 鈥斺€?椤圭洰鐗堟湰 SSOT
relkit stage
relkit inspect
relkit simulate
relkit verify
relkit publish
relkit backends
```

宸插疄鐜板悗绔細`s3-compatible` 路 `relkit-compatible` 路 `static-http`

### 蹇€熷紑濮?

```bash
relkit init --product demoapp
relkit keygen --key-id k1 --out keys --update-config
relkit version set 1.0.0+100
relkit stage --add dist/demoapp-win-x64.zip os=windows,arch=x64
relkit simulate --with-staged 1.0.0+100 --from all
relkit publish --dry-run
relkit publish
relkit verify --deep
```

`stage` / `publish` 鐪佺暐鐗堟湰鍙傛暟鏃惰 `VERSION.json`锛涢粯璁?`codeStrategy` 涓?`version-build`锛坈ode = `+build`锛夈€?
涓存椂鍙戝竷鏍戠粺涓€浣嶄簬 `.relkit/cache/staged/<version>/`锛涙秷璐归檮浠剁紦瀛樹綅浜?
`.relkit/cache/artifacts/`銆備骇鍝佷粨鍦?`.relkit/` 涓嬪彧搴旀彁浜?
`onboarding.json`锛宍.relkit/cache/` 蹇呴』蹇界暐銆?

### 鏇存柊鏃ュ織锛坈hangelog锛?

鍦?`relkit.json` 鍙€夐厤缃細

```json
"changelog": {
  "file": "Documents/changelog/CHANGELOG.md",
  "urlTemplate": "https://git.example.com/org/repo/blob/main/CHANGELOG.md#{anchor}"
}
```

- `stage`锛氭湭浼?`--notes` / `--notes-file` 鏃讹紝浠?`file` 鎶藉彇褰撳墠鐗堟湰灏忚妭锛圡arkdown锛夊啓鍏?`notes`锛沗urlTemplate`锛堟垨 `--notes-url`锛夌敓鎴?`notesUrl`
- `publish`锛氭渶鏂扮増鏈繚鐣欏唴鑱?`notes`锛涙洿鏃╃増鏈竻绌?`notes`銆佸彧鐣?`notesUrl`
- 鍗犱綅绗︼細`{version}` / `{version_slug}`锛坄+`鈫抈-`锛? `{anchor}`锛坄v`+slug锛?
- Dart SDK锛歚UpdateAvailable.releaseNotesMarkdown` / `releaseNotesUrl` / `priorReleaseNotes`

### 浜у搧椤典笌鍥哄畾涓嬭浇鍦板潃

浜у搧鍥㈤槦鍙湪 `relkit.json` 缁存姢缁欎汉鐪嬬殑鏂囨锛?

```json
"site": {
  "title": "Demo App",
  "description": "涓€鍙ヨ瘽璇存槑杩欎釜浜у搧鏄粈涔堛€佺粰璋佺敤銆?,
  "homepage": "https://git.example.com/org/demoapp",
  "makers": {
    "projectId": "makers-xxxxxxxx",
    "tokenEnv": "EDGEONE_PAGES_API_TOKEN"
  }
}
```

鍙戝竷瀹屾垚鍚庯紝relkit 杩樹細鍐欎袱绫?*涓嶅睘浜?RUP 淇′换閾?*鐨勭綉椤佃緟鍔╂寚閽堬細

- 姣忎釜 channel 鍙戝竷閮借鐩?`site/<product>.json`锛岀敱 `relkit-serve` 浜у搧闂ㄦ埛璇诲彇銆?
- 姣忎釜 channel 鍙戝竷鍙鐩栬嚜宸遍偅浠?`latest/<product>/<channel>.json`锛屽湪鍙戝竷鏃跺浐鍖栨湰鐗堝悇 artifact 鐨?ID銆乻electors 涓?URL銆俤ev 鍙戝竷涓嶅奖鍝?stable 鐨勬寚閽堛€?
- 鍏綉 COS 鍙戝竷鑻ラ厤浜?`site.makers`锛屽悓涓€杞繕浼氭妸 `.relkit/browse/` 閮ㄧ讲鍒?EdgeOne Makers锛圚TML 涓嶈繘 COS锛夈€傚唴缃?`relkit-compatible` 鎶婂悓涓€浠芥枃浠跺啓鍒版暟鎹潰 `browse/`銆?

鍥犳 `relkit-serve` 鍙寜 channel 鎻愪緵 `/-/latest/<product>/<channel>/<artifact-id>` 杩欑闀挎湡鏈夋晥鍦板潃锛屼緥濡?`/-/latest/demoapp/stable/windows`銆傝姹傚彧璇诲彇宸插彂甯冪殑 latest 鎸囬拡骞惰烦杞紝涓嶅疄鏃舵壂鎻?index / manifest銆?

## relkit-serve锛堝垎鍙戞湇鍔★級

```bash
relkit-serve init -dir /srv/releases -out /etc/relkit-serve
relkit-serve -config /etc/relkit-serve/relkit-serve.json
```

Linux + systemd锛?

```bash
sudo python3 deploy/relkit.py install serve --binary ./dist/relkit-serve-linux-amd64
```

宸叉湁瀹炰緥鍗囩骇锛歚python deploy/relkit.py upgrade --host <Host> --plan` 鐒跺悗 `--apply`銆傜粏鑺傝 [`deploy/README.md`](deploy/README.md)銆備骇鍝?token 鐢ㄤ骇鍝佷粨 `relkit_host.py serve`銆傝璁¤鏄庤 [`cmd/relkit-serve/README.md`](cmd/relkit-serve/README.md)銆?

## 璁捐涓庤鑼冩潵婧?

| 鍐呭 | 璺緞 |
|---|---|
| 鍗忚瑙勮寖 | [`SPEC.md`](SPEC.md) |
| 鍙戝竷宸ュ叿璁捐 | [`CLI.md`](CLI.md) |
| Protobuf 缁撴瀯 SSOT | [`proto/`](proto/)锛圙o / Dart锛氭敼瀹岃窇 `scripts/gen-proto.ps1`锛汵ode锛歚cd sdk/node && npm run generate`锛汻ust锛氭瀯寤烘椂鐢?vendored protoc 鐢熸垚锛?|
| JSON Schema锛堣緟鍔╋級 | [`schema/`](schema/) |
| 涓€鑷存€уす鍏?| [`conformance/`](conformance/) |
| 鍙戝竷渚ц繍缁?| 浜у搧浠?`python scripts/host/relkit_host.py` 路 [`skills/relkit-ops/SKILL.md`](skills/relkit-ops/SKILL.md) |
| 鍙戝竷鏈?agent | [`cmd/relkit-agent/README.md`](cmd/relkit-agent/README.md)銆乕`docs/design/publish-agent.md`](docs/design/publish-agent.md) |
| 瑁呮満 / 鎹簩杩涘埗 | [`deploy/README.md`](deploy/README.md) |

## 瀹㈡埛绔?SDK

鐩綍绾﹀畾瑙?[`sdk/README.md`](sdk/README.md)锛?

| | |
|--|--|
| Go | `relkit-sdk-go.zip` 闄勪欢瑁呭埌 `third_party/relkit`锛坄go list -deps` 绠楀嚭鐨勫彲缂栬瘧瀛愰泦锛夛紝閰?`replace go.firoyang.com/relkit => ./third_party/relkit` 路 [`sdk/README.md`](sdk/README.md) |
| Dart | `sdk/dart`锛坧ackage `rup_client`锛壜?[`sdk/dart/README.md`](sdk/dart/README.md) |
| Node | `sdk/node`锛坧ackage `rup-client`锛壜?[`sdk/node/README.md`](sdk/node/README.md) |
| Rust | `sdk/rust`锛坈rate `relkit-updater`锛屼緵 Tauri 澹宠皟鐢?sidecar锛壜?[`sdk/rust/README.md`](sdk/rust/README.md) |

```go
import "go.firoyang.com/relkit/sdk"

u := &sdk.Updater{
    Product: "myapp", Channel: "stable", CurrentCode: 100,
    IndexURLs: []string{"https://cdn.example.com/index/myapp/stable.pb"},
    TrustedKeys: sdk.TrustedKeys{"k1": pub},
    ClientSelectors: map[string]string{"os": "windows", "arch": "x64"},
}
result := u.Check(ctx)
```

Dart锛歚rup_client`锛坓it `path: sdk/dart`锛夈€?
Node锛歚rup-client`锛坄sdk/node`锛夈€?

## 寮€鍙戜笌娴嬭瘯

```bash
go test ./...
cargo test --manifest-path sdk/rust/Cargo.toml
cd sdk/node && npm test
```

瑕嗙洊锛?

- `chain` / `selectors` / `envelope` 鐨?conformance 澶瑰叿鍥炲綊
- `s3-compatible` / `relkit-compatible` 绔埌绔彂甯冿紝浠ュ強 `static-http` 鍙鏍￠獙锛坄.pb`锛?
- `relkit-serve` 鐨?Range / PUT / GC / 閰嶇疆鍔犺浇 / 鎿嶄綔闈㈡澘閴存潈
- `sdk` 瀹㈡埛绔?Check/Download锛圙o 涓?Node锛?
- `version` 椤圭洰 VERSION.json SSOT锛坓et/set/bump/code锛?
