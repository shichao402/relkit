# 鍙戝竷宸ュ叿璁捐锛堟殏鍚?`relkit`锛?

> 鐘舵€侊細璁捐绋匡紙璇箟浠嶉€傜敤锛夈€?*姝ｅ紡瀹炵幇鏄?Go 鍗曚簩杩涘埗**锛歔cnb.cool/shichao402/relkit](https://cnb.cool/shichao402/relkit)銆?
> 宸ュ叿鍚嶅緟瀹氾紝鏈枃涓€寰嬬敤 `relkit` 鍗犱綅

鏈枃鎻忚堪 RUP 鍗忚**鍙戝竷渚?*鐨勫懡浠や笌鍚庣璇箟銆傚鎴风杩愯鏃朵笉鍦ㄦ湰鏂囪寖鍥村唴 鈥斺€?鍚勫涓婚」鐩寜 `SPEC.md` 鑷瀹炵幇锛屽苟鐢?`conformance/` 楠岃瘉銆?

宸ュ叿涓庡崗璁槸瑙ｈ€︾殑锛歚SPEC.md` 涓嶆彁鍙婃湰宸ュ叿锛屾湰宸ュ叿涔熶笉寮曞叆浠讳綍 `SPEC.md` 涔嬪鐨勮涔夈€?

---

## 1. 纭害鏉?

1. **Go 鍗曚簩杩涘埗锛屾爣鍑嗗簱涓轰富銆?* 涓氬姟鏂?CI / 鏈満鐩存帴璺?`relkit`锛屼笉蹇?vendor 婧愮爜銆佷笉蹇呰嚜鍐欎笂浼犻€昏緫銆備粨搴擄細`cnb.cool/shichao402/relkit`銆?
2. 鑻ユ煇涓姛鑳界‘瀹為渶瑕侀澶栦緷璧栵紝**鍏堣皟鏁村姛鑳借璁?*锛涘彧鏈夊湪鍔熻兘涓嶅彲鏀惧純鏃讹紝鎵嶅紩鍏ュ苟鍦ㄦ枃浠跺ご娉ㄦ槑鐞嗙敱銆?
3. **鍙崟鏂囦欢鍒嗗彂銆?* `go build` / CNB Releases 澶氬钩鍙颁簩杩涘埗锛涗篃鍙敤 `go install cnb.cool/shichao402/relkit/cmd/relkit@latest`銆?
4. **Ed25519 鐢ㄨ繍琛屾椂鍘熺敓瀹炵幇**锛圙o锛歚crypto/ed25519`锛夈€傜鍚嶅璞℃槸 index payload 鐨勫師濮嬪瓧鑺傦紝瑙?SPEC.md 搂4銆?
5. **閰嶇疆鏂囦欢鐢?JSON**锛坄relkit.json`锛夛紝渚夸簬鎻愪氦杩涗粨搴撲笖璺ㄥ钩鍙颁竴鑷淬€傚嚟鎹彧閫氳繃鐜鍙橀噺鎴栨湰鏈虹閽ユ枃浠惰矾寰勪紶鍏ワ紝浠庝笉鍐欒繘閰嶇疆鍐呭銆?

---

## 2. 涓夋寮忔祦绋?

```
              鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€ 绾湰鍦帮紝绂荤嚎锛屽彲閲嶅 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
浜х墿鏂囦欢 鈹€鈹€鈻? relkit stage  鈹€鈹€鈻? .relkit/staged/<version>/
                                    staged.pb     鈫?鏈夊搱甯?澶у皬/鐗堟湰锛屾病鏈変换浣?URL
                                    artifacts/鈥?    鈫?浜х墿鍓湰
              鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?

              鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€ 鑱旂綉锛屾湁鍓綔鐢?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
staged 鈹€鈹€鈻?relkit publish 鈹€鈹€鈻?鈶?鏍￠獙鍙揪鎬э紙澶辫触鍗虫锛屼笉涓婁紶浠讳綍涓滆タ锛?
                              鈶?涓婁紶 artifact 鍒版墍鏈夊悗绔紝鏀堕泦 URL
                              鈶?鐢熸垚 manifest 瀛楄妭锛岀畻 sha256
                              鈶?涓婁紶 manifest 鍒版墍鏈夊悗绔紝鏀堕泦 URL
                              鈶?鏋勯€?index锛屽啀娆″畬鏁存牎楠?
                              鈶?绛惧悕 index
                              鈶?鍐?index 鎸囬拡  鈫?鎻愪氦鐐癸紝姝ゅ埢鐗堟湰鎵嶅瀹㈡埛绔彲瑙?
              鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
```

涓や釜闃舵涔嬮棿蹇呴』钀界洏锛岃繖鏄暣涓璁＄殑鏀偣锛?

- **stage 涓嶇煡閬撳悗绔殑瀛樺湪銆?* 鍥犳瀹冨彲绂荤嚎銆佸彲閲嶅銆佸彲鍗曞厓娴嬭瘯锛宍staged.pb` 涔熷彲浠ヨ瀹￠槄鐢氳嚦鎻愪氦杩涗粨搴撱€?
- **鍚屼竴浠?staged 鍙互鍙戝竷鍒颁换鎰忓涓悗绔?*锛屾棤闇€閲嶆柊鎵撳寘鎴栭噸绠楀搱甯屻€?
- 銆屼骇鐗╁埌搴曚笂缃戜簡娌℃湁銆嶆湁浜嗙‘瀹氱瓟妗堬細stage 涔嬪悗涓€瀹氭病鏈夈€?
- **鍙揪鎬ф牎楠屽湪涓婁紶涔嬪墠**銆備竴涓?`minFrom` 閰嶇疆閿欒浼氭案涔呴攣姝绘棫瀹㈡埛绔紝涓斾簨鍚庢棤娉曡ˉ鏁戯紝鎵€浠ュ繀椤诲湪浜х敓浠讳綍鍓綔鐢ㄤ箣鍓嶆嫤涓嬫潵銆?

---

## 3. 鍛戒护娓呭崟

| 鍛戒护 | 鑱旂綉 | 浣滅敤 |
|---|---|---|
| `relkit init` | 鍚?| 鐢熸垚閰嶇疆楠ㄦ灦 |
| `relkit keygen` | 鍚?| 鐢熸垚 Ed25519 瀵嗛挜瀵?|
| `relkit stage` | 鍚?| 鍥哄寲浜х墿涓庣増鏈俊鎭紝浜у嚭 `staged.pb` |
| `relkit inspect` | 鍚?| 浜虹被鍙鍦版墦鍗?staged / manifest / index |
| `relkit simulate` | 瑙嗘儏鍐?| 缁欏畾 index 涓庡綋鍓?code锛屾墦鍗板崌绾ц矾绾?|
| `relkit verify` | 鏄?| 鏍￠獙杩滅鐜扮姸锛氶獙绛俱€丼chema銆佸彲杈炬€с€佸搱甯屼竴鑷存€?|
| `relkit publish` | 鏄?| 瀹屾暣鍙戝竷娴佺▼ |
| `relkit yank` / `unyank` | 鏄?| 鎾ゅ洖 / 鍙栨秷鎾ゅ洖鏌愪釜鐗堟湰 |
| `relkit min-supported` | 鏄?| 璁剧疆寮哄埗鏇存柊涓嬮檺 |
| `relkit conformance` | 鍚?| 璺戜竴鑷存€х敤渚嬶紙鑷锛?|
| `relkit agent-guide` | 鍚?| 杈撳嚭鍐呭祵鐨?`AGENT-GUIDE.md` |
| `relkit backends` | 鍚?| 鍒楀嚭鏈鏋勫缓瀹為檯鏀寔鐨勫悗绔被鍨?|

---

## 4. 鍛戒护璇﹁В

### 4.1 `relkit keygen`

```
relkit keygen --key-id k1 --out keys/ --update-config
```

浜у嚭 `keys/k1.public.json`锛堝彲鎻愪氦杩涗粨搴擄紝涔熸槸瀹㈡埛绔鍐呭祵鐨勫唴瀹癸級涓?`keys/k1.private.json`锛?*绂佹**鎻愪氦锛夈€傜閽ユ枃浠舵潈闄愯涓?`0600`銆?

宸ュ叿**蹇呴』**鍦ㄦ娴嬪埌绉侀挜鏂囦欢浣嶄簬 git 宸ヤ綔鍖轰笖鏈 `.gitignore` 瑕嗙洊鏃剁粰鍑烘樉钁楄鍛娿€?

`--update-config` 鎶婂叕閽ョ洿鎺ュ啓杩涢厤缃殑 `signing.publicKeys`锛堝悓 `keyId` 鍒欐浛鎹級锛岀渷鎺変汉宸ュ鍒剁矘璐磋繖涓€姝?鈥斺€?瀹冨彧鍐欏叕閽ワ紝**涓嶄細**鍐?`privateKeyPath`锛屽洜涓哄摢鍙版満鍣ㄦ寔鏈夌閽ユ槸閭ｅ彴鏈哄櫒鑷繁鐨勪簨锛岃€岄厤缃槸瑕佹彁浜ょ殑銆?

### 4.2 `relkit stage`

```
relkit stage 1.5.0 \
  --code 150 \
  --min-from 120 \
  --notes-file NOTES.md \
  --add dist/app-win-x64.zip    os=windows,arch=x64 \
  --add dist/app-mac-arm64.zip  os=macos,arch=arm64 \
  --add dist/app-mac-x64.zip    os=macos,arch=x64 \
  --add dist/server.apk         target=server,os=android,kind=installer
```

`--add <璺緞> [k=v,...]`锛岄敭鍊煎垪琛ㄤ腑浠ヤ笅閿悕涓轰繚鐣欐帶鍒跺瓧娈碉紝鍏朵綑涓€寰嬩綔涓?`selectors`锛?

| 淇濈暀閿?| 浣滅敤 | 缂虹渷 |
|---|---|---|
| `id` | artifact 鐨?`id` | 鐢?selectors 鐨勫€兼嫾鎺ワ細鏍囧噯閿寜 SPEC.md 搂11.1 鐨勮〃鏍奸『搴忥紙`os` `arch` `target` `abi` `variant`锛夊湪鍓嶏紝鑷畾涔夐敭鎸夐敭鍚嶆帓搴忓湪鍚庯紝濡?`windows-x64`锛涙棤 selectors 鏃朵负 `default` |
| `kind` | artifact 鐨?`kind` | 鎸夋墿灞曞悕鎺ㄦ柇锛岃涓?|
| `filename` | 钀界洏鏂囦欢鍚?| 婧愭枃浠剁殑 basename |
| `meta.*` | 鍐欏叆 `meta` 瀵硅薄 | 鏃?|

`kind` 鐨勬墿灞曞悕鎺ㄦ柇锛歚.zip` `.tar.gz` `.tgz` 鈫?`archive`锛沗.exe` `.msi` `.dmg` `.pkg` `.apk` `.deb` `.rpm` 鈫?`installer`锛涙棤鎵╁睍鍚?鈫?`binary`锛涘叾浣?鈫?`blob`銆傛帹鏂粨鏋?*蹇呴』**鎵撳嵃鍑烘潵锛岄伩鍏嶉潤榛樼寽閿欍€?

`--code` 鐨勫彇鍊肩瓥鐣ョ敱閰嶇疆鐨?`codeStrategy` 鍐冲畾锛?

| 绛栫暐 | 琛屼负 |
|---|---|
| `explicit`锛堥粯璁わ級 | 蹇呴』鏄惧紡浼?`--code`锛屼笉浼犲垯鎶ラ敊 |
| `semver` | 浠?`version` 鎺ㄥ锛歚major*1000000 + minor*1000 + patch`锛堝 `1.13.21` 鈫?`1013021`锛夈€俙minor` 鎴?`patch` 杈惧埌 1000 浼氳繘浣嶄覆鍙凤紝宸ュ叿**蹇呴』**鎷掔粷 |
| `version-build` | 鍙?`version` 鐨?`+build` 娈碉紙濡?`0.0.21+7` 鈫?`7`锛?|
| `env:VAR` | 鍙栫幆澧冨彉閲忥紝閫氬父鏄?CI 鐨勬瀯寤哄彿 |
| `env:VAR+N` | 鍙栫幆澧冨彉閲忓啀鍔犲浐瀹氬亸绉婚噺 |

娲剧敓鍨嬬瓥鐣ワ紙闄?`explicit` 澶栵級涓嬩紶鍏ョ殑 `--code` 鍙仛**浜ゅ弶鏍￠獙**锛氫笌鎺ㄥ鍊间笉涓€鑷存椂鎶ラ敊鑰岄潪瑕嗙洊銆傚惁鍒欎竴涓墜婊戠殑鍙傛暟灏辫兘缁曡繃椤圭洰澹版槑鐨勭瓥鐣ワ紝鑰岃繖姝ｆ槸 `code` 鍗曡皟鎬ц鐮村潖鐨勫父瑙侀€斿緞銆?

`version-build` 鏄?Flutter / pubspec 椋庢牸椤圭洰鐨勯閫夛細鏋勫缓鍙锋湰鏉ュ氨鍐欏湪鏉冨▉鐗堟湰鏂囦欢閲屻€佹湰鏉ュ氨鍗曡皟锛岃€屼笖瀹㈡埛绔繍琛屾椂鑳借鍒板悓涓€涓€硷紝鍥犳銆屽鎴风鑷姤鐨?code銆嶄笌銆屽彂甯冩柟鍒嗛厤鐨?code銆嶄笉浼氭紓绉?鈥斺€?杩欐槸 `env:VAR` 鍋氫笉鍒扮殑锛岀幆澧冨彉閲忓彧瀛樺湪浜庢瀯寤烘満涓娿€?

**浣跨敤 CI 鏋勫缓鍙锋椂鍔″繀娉ㄦ剰閲嶇疆椋庨櫓銆?* `GITHUB_RUN_NUMBER` 涔嬬被鐨勮鏁板櫒鍦ㄦ洿鎹?CI 骞冲彴銆侀噸寤轰粨搴撱€佹垨鎶?workflow 鏂囦欢鏀瑰悕鏃朵細閫€鍥?1銆傚崗璁姹?`code` 涓ユ牸鍗曡皟閫掑锛屼竴鏃﹀€掗€€锛屾柊鍙戝竷鐨勭増鏈湪瀹㈡埛绔湅鏉ヤ細銆屾瘮宸茶鐨勬棫銆嶏紝鍚庢灉鏄墍鏈変汉閮芥敹涓嶅埌鏇存柊锛岃€屼笖杩欎釜鏁呴殰闈炲父涓嶇洿瑙傘€?

涓ら亾闃茬嚎锛氫竴鏄?`env:VAR+N` 鐨勫亸绉婚噺锛岃縼绉绘椂鎶婂亸绉绘姮鍒拌秴杩囧巻鍙叉渶澶у€煎嵆鍙户缁崟璋冿紱浜屾槸涓嬮潰 搂4.5 绗?3 姝ョ殑寮哄埗鏍￠獙锛屽畠鍦ㄤ笂浼犱换浣曚笢瑗夸箣鍓嶅氨浼氭嫤涓嬪€掗€€鐨?`code`銆?

**stage 蹇呴』鎵ц鐨勬湰鍦版牎楠岋細**

1. 鍚屼竴娆?stage 鍐?`id` 鍞竴锛?
2. 鍚屼竴娆?stage 鍐呬笉瀛樺湪涓や釜 `selectors` 瀹屽叏鐩稿悓鐨?artifact锛圫PEC.md 搂11 瑕佹眰鍙戝竷渚т繚璇佽繖涓€鐐癸級锛?
3. `filename` 閫氳繃 SPEC.md 搂14.4 鐨勫畨鍏ㄦ鏌ワ紱
4. 浜х墿鏂囦欢瀛樺湪涓斿彲璇伙紝璁＄畻骞惰褰?`sha256` 涓?`size`銆?

浜х墿榛樿**鎷疯礉**鍒?`.relkit/staged/<version>/artifacts/`銆傘€屽浐鍖栥€嶆槸杩欎竴姝ョ殑鏍稿績璇箟锛屾嫹璐濊 staged 鐩綍鑷冻锛屼箣鍚庢簮鐩綍琚竻鐞嗘垨閲嶆柊鏋勫缓閮戒笉褰卞搷鍙戝竷銆傚悓鐩樻椂鍙敤 `--link` 鏀逛负纭摼鎺ヤ互鐪佺┖闂淬€?

`.relkit/` **搴旇**鍔犲叆 `.gitignore`銆?

### 4.3 `relkit simulate`

璋冭瘯 `minFrom` 閰嶇疆鐨勪富瑕佹墜娈点€?

```
relkit simulate --index dist/publish/index/myapp/stable.json --from 100
relkit simulate --from all                        # 涓嶇粰 --index 鏃朵粠杩滅鎷夊彇褰撳墠 index
relkit simulate --with-staged 1.5.0 --from all    # 鎶婂緟鍙戝竷鐗堟湰骞跺叆鍚庡啀妯℃嫙
```

index 鏉ユ簮涓夐€変竴锛歚--index` 鎸囧畾鏈湴鏂囦欢锛涗笉鎸囧畾鍒欐寜閰嶇疆浠庤繙绔媺鍙栵紙姝ゆ椂鑱旂綉锛夛紱`--with-staged <version>` 鍦ㄤ笂杩板熀鍑嗕箣涓婂苟鍏?`staged.pb` 鎻忚堪鐨勬柊鑺傜偣銆?

`--with-staged` 鏄彂甯冨墠璋冭瘯 `minFrom` 鍞竴鏈夋剰涔夌殑褰㈠紡 鈥斺€?寰呭彂甯冪殑鑺傜偣杩樹笉瀛樺湪浜庝换浣?index 涓紝鍙湅鍩哄噯 index 鏄湅涓嶅埌鏈鏀瑰姩鐨勫奖鍝嶇殑銆傚畠涓?`publish --dry-run` 鐨勫垎宸ユ槸锛氬悗鑰呰礋璐?*鎷︽埅**锛堟牎楠屼笉閫氳繃灏变腑姝級锛屽墠鑰呰礋璐?*灞曠ず**锛堟妸姣忎釜璧风偣鐨勮惤鐐规憡寮€锛夛紝鍥犳鍗充娇鏍￠獙鑳介€氳繃涔熶粛鏈変环鍊笺€俙--with-staged` 涓嶅啓浠讳綍涓滆タ锛屼篃涓嶈姹傜閽ャ€?

杈撳嚭锛?

```
current code 100
  hop 1  ->  1.2.0  (code 120)   minFrom=100   閰嶇疆鏂囦欢鏍煎紡鍙樻洿锛屾洿鏃╃殑鐗堟湰蹇呴』缁忕敱鏈増鏈崌绾?
  hop 2  ->  1.5.0  (code 150)   minFrom=120
reachable head: 1.5.0
mandatory: no
```

`--from all` 閬嶅巻鎵€鏈夎妭鐐?code 鍔犱笂 `0`锛屾墦鍗版瘡涓捣鐐圭殑瀹屾暣璺嚎銆傝繖鏄湪鍔ㄦ墜鏀?`minFrom` 涔嬪墠搴旇鍏堢湅涓€鐪肩殑涓滆タ 鈥斺€?瀹冩妸銆岃皝浼氳鍗′綇銆嶇洿鎺ユ憡寮€銆?

### 4.4 `relkit verify`

瀵硅繙绔幇鐘跺仛鍙妫€鏌ワ紝浠讳綍浜洪兘鑳借窇锛堟棤闇€绉侀挜銆佹棤闇€鍐欐潈闄愶級锛?

1. 閫愪釜鍚庣鎷夊彇 index锛岀敤鍏挜楠岀锛?
2. 鏍￠獙 Schema锛?
3. 鎵ц SPEC.md 搂10 鐨勫彲杈炬€ф牎楠岋紱
4. 鎶藉彇姣忎釜鐗堟湰鑺傜偣锛屼笅杞?manifest锛屾瘮瀵?`sha256` 涓?`size`锛?
5. 鍙€?`--deep`锛氬姣忎釜 artifact 鍙?HEAD 璇锋眰锛岀‘璁ゅ彲鍖垮悕璁块棶涓?`Content-Length` 涓?`size` 涓€鑷达紙涓嶄笅杞藉畬鏁村唴瀹癸級锛?
6. 姣斿鍚勫悗绔殑 index 鏄惁瀛楄妭鐩稿悓锛屼笉鍚屽垯鎶ュ憡鍝釜钀藉悗锛坄sequence` 杈冨皬锛夈€?

绗?5 姝ユ槸鍥炵瓟銆屽鎴风鍒板簳涓嬩笉涓嬫潵銆嶇殑鏈€鐩存帴鎵嬫锛屽挨鍏堕€傜敤浜庡悗绔尶鍚嶈闂瓥鐣ヤ笉鏄庣‘鐨勬儏鍐碉紙渚嬪 CNB Release 闄勪欢锛岃 SPEC.md 搂13.2锛夈€?

### 4.5 `relkit publish`

```
relkit publish 1.5.0                    # 鍙戝竷鍒伴厤缃腑 publishTo 鍒楀嚭鐨勬墍鏈夊悗绔?
relkit publish 1.5.0 --to local         # 鍙骇鍑哄埌鏈湴鐩綍
relkit publish 1.5.0 --dry-run          # 鍙仛鏍￠獙骞舵墦鍗拌鍒掞紝涓嶄骇鐢熶换浣曞壇浣滅敤
relkit publish 1.5.0 --allow-partial    # 鍏佽閮ㄥ垎鍚庣澶辫触
```

涓ユ牸鎸変互涓嬮『搴忥紝缂栧彿鍗虫墽琛岄『搴忥細

1. 璇诲彇 `staged.pb`锛?*閲嶆柊璁＄畻**姣忎釜浜х墿鐨?`sha256` 骞朵笌璁板綍姣斿銆俿taged 鐩綍鍙兘鍦?stage 涔嬪悗琚敼鍔ㄨ繃锛屼笉閲嶇畻灏辩瓑浜庝俊浠讳竴涓彲鑳藉凡缁忓け鏁堢殑鍝堝笇銆?
2. 浠庢瘡涓悗绔媺鍙栫幇鏈?index 骞堕獙绛俱€傚彇鎵€鏈夊悗绔腑鏈€澶х殑 `sequence` 浣滀负鍩哄噯锛屾柊 `sequence` = 鍩哄噯 + 1銆傝嫢鍚勫悗绔殑 index 鍐呭涓嶄竴鑷达紝榛樿涓骞舵彁绀哄厛璺?`relkit verify`銆?
3. 鏍￠獙鏂扮増鏈殑 `code` **涓ユ牸澶т簬**杩滅 index 涓墍鏈夌幇鏈?`code`锛岀劧鍚庢妸鏂拌妭鐐瑰苟鍏ョ増鏈垪琛紝鎵ц SPEC.md 搂10 鐨?*鍏ㄩ儴鏍￠獙**銆傛湁閿欒鍒?*绔嬪嵆涓锛屼笉涓婁紶浠讳綍涓滆タ**銆傝繖涓€姝ヤ綅缃潬鍓嶆槸鏈夋剰鐨勩€?

   鍗曡皟鎬т箣鎵€浠ヨ鍗曠嫭妫€鏌ワ細寰€閾炬潯涓棿鎻掑叆涓€涓?`code` 鏇村皬鐨勮妭鐐癸紝鍦ㄨ娉曚笌鍙揪鎬т笂閮藉彲鑳藉畬鍏ㄥ悎娉曪紝浣嗗畠鎰忓懗鐫€銆屽彂甯冧簡涓€涓瘮鐜版湁鏈€鏂扮増鏇存棫鐨勭増鏈€嶏紝鍑犱箮鎬绘槸 `code` 鐨勬潵婧愬嚭浜嗛棶棰橈紙鏈€甯歌鐨勫氨鏄?CI 鏋勫缓鍙疯閲嶇疆锛岃 搂4.2锛夈€傛瀬灏戞暟纭疄闇€瑕佽ˉ褰曞巻鍙茬増鏈殑鍦烘櫙鍙敤 `--allow-backfill` 鏄惧紡鏀捐銆?
4. 涓婁紶鎵€鏈?artifact 鍒版墍鏈夌洰鏍囧悗绔紝鏀堕泦姣忎釜 artifact 鐨勫叏閮?URL銆?
5. 鐢熸垚 manifest 瀛楄妭锛堟瘡涓?artifact 鐨?`urls` 鍖呭惈**鎵€鏈?*鍚庣鐨?URL锛夛紝璁＄畻 `sha256` 涓?`size`銆?
6. 涓婁紶 manifest 鍒版墍鏈夌洰鏍囧悗绔紝鏀堕泦鍏跺叏閮?URL銆?
7. 鏋勯€犲畬鏁?index锛氬～鍏?`manifest.sha256` / `size` / `urls`锛屽啓鍏ユ柊 `sequence`銆?
8. 鍐嶆鎵ц瀹屾暣鏍￠獙锛岃繖娆″寘鍚搱甯屼竴鑷存€с€?
9. 鐢ㄧ閽ョ鍚嶏紝鐢熸垚淇″皝锛岄殢鍗?*鐢ㄩ厤缃噷鐨?`signing.publicKeys` 鎶婅繖浠芥柊绛惧悕楠屼竴閬?*銆?

   杩欎竴姝ユ嫤鐨勬槸鍞竴浼氥€屽彂甯冩垚鍔熶絾瀹㈡埛绔叏鐏€嶇殑鏁呴殰锛氱鍚嶇敤鐨?`keyId` 涓嶅湪瀹㈡埛绔俊浠荤殑闆嗗悎閲屻€傛鏃朵笂浼犮€侀摼鏉°€佸搱甯屽叏閮芥纭紝鎸囬拡涔熷啓鎴愬姛浜嗭紝浣嗘瘡涓鎴风閮戒細鎸?SPEC.md 搂12.1 闈欓粯鎷掔粷杩欎唤 index 鈥斺€?鑰?SPEC 瑕佹眰瀹㈡埛绔矇榛橈紝鎵€浠ョ幇鍦鸿〃鐜版槸銆屾洿鏂版湇鍔″櫒鍧忎簡銆嶏紝鑰屼笉鏄€岄厤缃啓閿欎簡銆嶃€傛渶甯歌鐨勬垚鍥犳槸 `relkit init` 棰勫～鐨勫崰浣?`keyId` 涓€鐩存病鏀癸紝鑰岀湡瀹炲瘑閽ヤ互鍙︿竴涓悕瀛楄褰曞湪 `publicKeys` 閲屻€傚彂甯冩柟鏃㈢劧鎵嬩笂灏辨湁淇′换闆嗭紝灏辨病鏈夌悊鐢变笉鍦ㄦ彁浜ょ偣涔嬪墠鑷繁楠屼竴娆°€?
10. 鎶婁俊灏佸啓鍏ユ墍鏈夌洰鏍囧悗绔殑鎸囬拡浣嶇疆銆?*杩欐槸鎻愪氦鐐广€?*

鍏充簬澶辫触锛?

- 绗?10 姝ヤ箣鍓嶇殑浠讳綍澶辫触閮芥槸瀹夊叏鐨?鈥斺€?宸蹭笂浼犵殑 artifact 涓?manifest 瀵瑰鎴风涓嶅彲瑙侊紝鍥犱负 index 杩樻病鏇存柊銆傞噸璺戝嵆鍙紝瀛ゅ効鏂囦欢鍙敤 `relkit gc`锛堝悗缁€冭檻锛夋竻鐞嗐€?
- 绗?10 姝ラ儴鍒嗘垚鍔熸椂锛屾湭鏇存柊鐨勫悗绔粛鎻愪緵鏃?index銆傚鎴风浠庤鍚庣鍙栧埌杈冨皬鐨?`sequence`锛屾寜 SPEC.md 搂12.4 浼氳褰撲綔銆屾湰娆℃棤鏇存柊銆嶈€屼笉鏄敊璇紝鍥犳涓嶄細鏁呴殰锛岀瓑鎸囬拡琛ラ綈鍚庤嚜鐒舵仮澶嶃€傝繖涔熸槸 `--allow-partial` 涔嬫墍浠ュ畨鍏ㄧ殑鍘熷洜銆?

**鎵€鏈夊悗绔殑 index 涓?manifest 蹇呴』瀛楄妭瀹屽叏鐩稿悓銆?* index 鏄鍚嶅璞★紝鍐呭涓嶅悓灏辨剰鍛崇潃瑕佺澶氭銆佺淮鎶ゅ浠姐€佸苟鍙兘涓嶄竴鑷淬€傞暅鍍忕殑宸紓鍙綋鐜板湪 `urls` 鏁扮粍澶氫簡鍑犱釜鍏冪礌涓婏紙SPEC.md 搂5.3锛夈€傝繖姝ｆ槸鏈伐鍏蜂笌 RemoteCam 鐜版湁 `generate_update_config.py` + `convert_config_to_gitee.py` 涓よ剼鏈柟妗堢殑鍏抽敭鍖哄埆锛氬悗鑰呬负 GitHub 鍜?Gitee 鍚勭敓鎴愪竴浠藉唴瀹逛笉鍚岀殑娓呭崟銆?

### 4.6 `relkit yank` / `unyank` / `min-supported`

```
relkit yank 1.5.0 --reason "鍚姩宕╂簝"
relkit min-supported 120
```

涓夎€呴兘鏄€岃鍙栬繙绔?index 鈫?淇敼 鈫?閲嶆柊鏍￠獙 鈫?`sequence` + 1 鈫?閲嶆柊绛惧悕 鈫?鍐欐寚閽堛€嶏紝涓嶆秹鍙婁骇鐗╀笂浼犮€?

`yank` **蹇呴』**閲嶆柊鎵ц鍙揪鎬ф牎楠岋細鎾ゅ洖涓€涓妭鐐逛細鏀瑰彉 head锛屼篃鍙兘鍒囨柇鍏朵粬鑺傜偣鐨勫崌绾ц矾寰勩€傝嫢鎾ゅ洖瀵艰嚧鏍￠獙澶辫触锛屽伐鍏?*蹇呴』**鎷掔粷骞惰鏄庢槸鍝簺鐗堟湰浼氳鍗′綇 鈥斺€?鎾ゅ洖涓€涓潖鐗堟湰鏃舵渶涓嶉渶瑕佺殑灏辨槸椤烘墜鎶婃墍鏈変汉閿佹銆?

`yank` **绂佹**鍒犻櫎鑺傜偣锛屽彧缃?`yanked: true`锛圫PEC.md 搂9.4锛夈€?

### 4.7 `relkit conformance`

璺?`conformance/` 涓嬬殑鍏ㄩ儴鐢ㄤ緥銆傚伐鍏疯嚜韬殑閫夎矾銆佸彲杈炬€с€侀€夋嫨鍣ㄣ€侀獙绛惧疄鐜?*蹇呴』**涓庣敤渚嬩竴鑷?鈥斺€?鍙戝竷渚у拰瀹㈡埛绔叡鐢ㄥ悓涓€濂楀垽瀹氾紝鑻ヤ袱杈圭悊瑙ｄ笉鍚岋紝鍙戝竷渚у氨浼氭斁琛屼竴涓鎴风瀹為檯澶勭悊涓嶄簡鐨?index銆?

### 4.8 `relkit agent-guide`

鎶?`AGENT-GUIDE.md` 鍘熸枃鎵撳嵃鍒?stdout銆?

瀛樺湪鐨勭悊鐢憋細`AGENT-GUIDE.md` 鏄搷浣滄€х煡璇嗙殑 SSOT锛岃€岃皟鐢ㄥ畠鐨?skill 浼氳鎷疯繘鍚勪釜浣跨敤鏂归」鐩紝灞婃椂鎸囧悜鏈粨搴撶殑鐩稿璺緞灏辨柇浜嗐€傛妸鏂囨。闅忓彲鎵ц鏂囦欢涓€璧峰垎鍙戯紝浣?寮曠敤 SSOT"鍦ㄤ换浣曢」鐩噷閮芥垚绔嬶紝涓斾笉浜х敓浼氬悇鑷紓绉荤殑鍓湰銆?

鍥犳鏋勫缓鏃?*蹇呴』**鎶?`AGENT-GUIDE.md` 鍘熸牱宓屽叆浜х墿锛?*绂佹**鍦ㄥ祵鍏ヨ繃绋嬩腑鏀瑰啓鍐呭銆?

---

## 5. 閰嶇疆鏂囦欢

椤圭洰鏍圭洰褰?`relkit.json`锛?

```json
{
  "product": "myapp",
  "defaultChannel": "stable",
  "channels": ["stable", "beta"],
  "codeStrategy": "explicit",

  "signing": {
    "keyId": "k1",
    "privateKeyPath": ".relkit-keys/k1.private.pb",
    "publicKeys": [
      { "keyId": "k1", "publicKeyBase64": "OKreeDPBlpB/FAYzpTZW2HPNrwI9qd+8AJNimIBk8Pk=" }
    ]
  },

  "backends": {
    "github": {
      "type": "github-release",
      "repo": "owner/myapp",
      "tokenEnv": "GITHUB_TOKEN",
      "pointerTag": "channel-{channel}",
      "versionTag": "v{version}"
    },
    "cnb": {
      "type": "cnb-release",
      "repo": "group/myapp",
      "apiEndpoint": "https://api.cnb.cool",
      "tokenEnv": "CNB_TOKEN",
      "pointerTag": "channel-{channel}",
      "versionTag": "v{version}"
    },
    "cos": {
      "type": "s3-compatible",
      "endpoint": "https://cos.ap-guangzhou.myqcloud.com",
      "bucket": "myapp-release",
      "prefix": "rup/",
      "baseUrl": "https://dl.example.com/rup/",
      "accessKeyEnv": "COS_SECRET_ID",
      "secretKeyEnv": "COS_SECRET_KEY"
    },
    "local": {
      "type": "local",
      "outputDir": "dist/publish",
      "baseUrl": "https://dl.example.com/"
    }
  },

  "site": {
    "title": "myapp",
    "makers": {
      "projectId": "makers-xxxxxxxx",
      "tokenEnv": "EDGEONE_PAGES_API_TOKEN"
    }
  },

  "publishTo": ["github", "cos"]
}
```

鍑嵁**鍙?*閫氳繃鐜鍙橀噺浼犲叆锛坄*Env` 瀛楁缁欏嚭鍙橀噺鍚嶏級銆傞厤缃枃浠舵湰韬彲浠ユ彁浜よ繘浠撳簱锛?*绂佹**鍦ㄥ叾涓嚭鐜颁换浣?token銆佸瘑閽ユ垨绉侀挜鍐呭銆俙privateKeyPath` 浠呬緵鏈満浣跨敤锛屾寚鍚戠殑鏂囦欢**绂佹**杩涘叆浠撳簱銆?

`signing.publicKeys` 鍒楀嚭鍙椾俊鍏挜锛屾鏄鎴风瑕佸唴宓岀殑閭ｄ唤鍐呭銆傚畠瀛樺湪鐨勭悊鐢辨槸璁?`relkit verify` **涓嶉渶瑕佺閽?*锛氫换浣曡兘璇诲埌閰嶇疆鐨勪汉閮借兘瀹¤涓€娆″彂甯冿紝鏃犻渶鍏峰鍙戝竷鏉冮檺銆傝疆鎹㈡湡鍐呰繖閲屼細鍚屾椂鍒楀嚭鏂版棫涓ゆ妸鍏挜锛屼笌 SPEC.md 搂4.1 鐨勫绛惧悕鏉＄洰鐩稿搴斻€?

鍙戝竷鏃舵棤闇€閰嶇疆瀹?鈥斺€?绉侀挜鑳芥帹瀵煎嚭鍏挜銆備絾鏍￠獙鏃跺繀椤绘湁銆?

---

## 6. 鍚庣鎻掍欢

鎺ュ彛鍗?SPEC.md 搂13 鐨勫洓涓柟娉曪紙Go 绀烘剰锛夛細

```go
type Backend interface {
    PutArtifact(localPath, key string) ([]string, error)
    PutImmutable(data []byte, key string) ([]string, error)
    PutPointer(data []byte, key string) ([]string, error)
    Get(key string) ([]byte, error) // absent -> (nil, nil)
    URLFor(key string) (string, bool)
}
```

`Get` 鐢ㄤ簬璇诲洖鐜扮姸锛歚publish` 绗?2 姝ヨ璇荤幇鏈?index锛宍verify` 瑕佽 index 涓庡叏閮?manifest銆傚畠**涓嶈兘**鏇挎崲鎴愩€屾寜鏂囨。閲岀殑 URL 鍘讳笅杞姐€嶏紝鍥犱负 `local` 鍚庣鐨?`baseUrl` 鎻忚堪鐨勬槸銆屽皢鏉ュ噯澶囨墭绠″湪鍝€嶏紝姝ゅ埢閫氬父骞朵笉鍙В鏋愩€?

`URLFor` 鏄矾寰勫瀷鍚庣鎵嶆湁鐨勮兘鍔涳細URL 鍦ㄤ笂浼犲墠灏辫兘绠楀嚭鏉ャ€俁elease 鍨嬭繑鍥?false銆俙verify` 鐢ㄥ畠鍋氫竴椤瑰鏄撹蹇界暐鐨勬鏌?鈥斺€?鍚庣涓烘煇涓?key 鐢熸垚鐨?URL 鏄惁鐪熺殑鍑虹幇鍦ㄥ凡鍙戝竷鐨勬枃妗ｉ噷銆備笉涓€鑷存剰鍛崇潃 `baseUrl` 鏀硅繃浣嗘病鏈夐噸鏂板彂甯冿紝姝ゆ椂鏂囦欢鍦ㄦ柊浣嶇疆銆佹枃妗ｆ寚鍚戞棫浣嶇疆锛屽鎴风浼氶潤榛樺湴涓嬭浇澶辫触銆?

### 6.1 涓ょ被鍚庣

鎸?SPEC.md 搂13.1锛屽悗绔垎鎴愪袱绫伙紝鍒嗙晫鍐冲畾浜嗗疄鐜拌兘涓嶈兘鎷嗭細

**璺緞鍨?*鍏辩敤 `PathStyleBackend`锛屽畠瀹炵幇 URL 鎺ㄥ锛坄baseUrl + quote(key)`锛変笌杈撳嚭璺緞鐨勮秺鐣屼繚鎶ゃ€傚瓙绫诲彧闇€瀹炵幇銆屾€庝箞鎶婂瓧鑺傛斁涓婂幓銆嶏細

| type | 鏀剧疆鏂瑰紡 | 鐘舵€?|
|---|---|---|
| `local` | 鍐欐湰鍦扮洰褰?| **宸插疄鐜?* |
| `static-http` | 鍐欐湰鍦扮洰褰曪紝浜ょ粰浠撳簱 CI / rsync / 涓婁紶姝ラ鎺ユ墜锛涗笉閰嶅垯鍙 | **宸插疄鐜?* |
| `http-put` | 甯﹂壌鏉冪殑 PUT 涓婁紶锛堥厤 relkit-serve锛屾垨浠讳綍 PUT / WebDAV 绔偣锛?| **宸插疄鐜?* |
| `s3-compatible` | COS / S3 / MinIO，SigV4 签名 | **已实现** |

**Release 鍨?*蹇呴』鏁翠綋瀹炵幇锛屽洜涓?URL 鐢变笂浼犲搷搴旇繑鍥烇細

| type | 璇存槑 | 鐘舵€?|
|---|---|---|
| `github-release` | GitHub Release asset | 鏈疄鐜?|
| `cnb-release` | CNB Release 闄勪欢锛岃蛋 Open API | 鏈疄鐜帮紝涓旈渶鍏堥獙璇佸尶鍚嶄笅杞斤紙瑙?搂6.2锛?|
| `gitee-release` | Gitee Release asset | 鏈疄鐜?|

`local` 鏈€鍏堝疄鐜帮紝鍥犱负瀹冭宸ュ叿绔嬪埢鍙敤浜庝换浣曢潤鎬佹墭绠℃柟寮忥紝涓嶈浠讳綍鍗曚竴骞冲彴鐨勮兘鍔涙垨鏀跨瓥鍗′綇锛屼篃鏄鍒扮娴嬭瘯鐨勫熀纭€ 鈥斺€?鏁翠釜鍙戝竷娴佺▼鍙互瀹屽叏绂荤嚎璺戦€氥€?

### 6.2 `static-http`

瑕嗙洊涓€鍒囥€屾寜鍙娴嬭矾寰勬彁渚?HTTP 涓嬭浇銆嶇殑鎵樼锛欳NB 浠撳簱鐩撮摼銆佸璞″瓨鍌ㄦ寕鍩熷悕銆丯ginx銆丟itHub Pages銆丆DN 鍥炴簮銆備粠瀹㈡埛绔湅瀹冧滑姣棤鍖哄埆锛岄兘鍙槸涓€娆?HTTP GET锛屾墍浠ュ畠浠槸鍚屼竴涓悗绔€?

```json
{
  "type": "static-http",
  "baseUrl": "https://cnb.cool/group/repo/-/raw/main/release/",
  "stageDir": "release",
  "timeoutSeconds": 30
}
```

| 瀛楁 | 蹇呭～ | 璇存槑 |
|---|---|---|
| `baseUrl` | 鏄?| 缁濆 http(s) URL锛屾湯灏炬枩鏉犲彲鐪佺暐銆俇RL 鍗?`baseUrl + key` |
| `stageDir` | 鍚?| 鐩稿椤圭洰鏍圭殑鐩綍锛屽彂甯冩椂鎶?key 鐩綍鏍戝啓杩涘幓銆?*涓嶉厤鍒欏悗绔彉涓哄彧璇?* |
| `timeoutSeconds` | 鍚?| 鍗曟璇锋眰瓒呮椂锛岄粯璁?30 |

`stageDir` 灏辨槸 SPEC.md 搂13.1 閲岄偅涓€鍒椼€屾斁缃€嶇殑鏈€绠€瀹炵幇锛氭枃浠惰惤杩涘伐浣滃尯锛屽墿涓嬬殑浜ょ粰浠撳簱 CI锛圕NB 鍦烘櫙锛夈€乺sync 浠诲姟鎴栫嫭绔嬬殑涓婁紶姝ラ銆?*CNB 鍥犳涓嶉渶瑕佷换浣曚笂浼犲疄鐜?* 鈥斺€?浜х墿闅忎粨搴撴祦杞槸瀹冩湰鏉ュ氨鏈夌殑鑳藉姏銆?

涓嶉厤 `stageDir` 寰楀埌涓€涓彧璇诲悗绔紝鐢ㄩ€旀槸瀹¤锛氭寚鍚戝埆浜哄彂甯冪殑绔欑偣璺?`relkit verify`锛屼笉闇€瑕佸嚟鎹篃涓嶉渶瑕佸啓鏉冮檺銆傛鏃?`publish` 浼氬湪**浠讳綍缃戠粶璇诲彇涔嬪墠**灏辨嫆缁濓紝鍥犳 `--dry-run` 鐨勭粨璁轰笌鐪熷疄鍙戝竷涓€鑷淬€?

### 6.3 `http-put`

涓嬭浇渚т笌 `static-http` 瀹屽叏涓€鑷?鈥斺€?鍚屾牱鐨?URL銆佸悓鏍风殑鏍￠獙銆佸悓鏍风殑瀹㈡埛绔涓?鈥斺€?鍙湁鏀剧疆涓嶅悓锛氫笉鏄啓杩涚洰褰曠瓑鍒汉鎼繍锛岃€屾槸 PUT 鍒颁竴涓鐐广€傞厤鍚屼粨鐨?`relkit-serve`锛坄cmd/relkit-serve`锛夋垨浠讳綍瀹炵幇鏍囧噯 PUT 璇箟鐨勬湇鍔★紙鍚?WebDAV锛夈€?

```json
{
  "type": "http-put",
  "baseUrl": "https://cdn.example.com/releases/",
  "uploadUrl": "http://10.0.0.5:8080/",
  "tokenEnv": "RELKIT_UPLOAD_TOKEN",
  "timeoutSeconds": 600
}
```

| 瀛楁 | 蹇呭～ | 璇存槑 |
|---|---|---|
| `baseUrl` | 鏄?| 瀹㈡埛绔笅杞界敤鐨勫熀鍦板潃锛孶RL 鍗?`baseUrl + key` |
| `uploadUrl` | 鍚?| PUT 鐨勭洰鏍囧熀鍦板潃锛岄粯璁ょ瓑浜?`baseUrl` |
| `tokenEnv` | 鏄?| 瀛樻斁涓婁紶 token 鐨勭幆澧冨彉閲忓悕 |
| `timeoutSeconds` | 鍚?| 涓婁紶瓒呮椂锛岄粯璁?600锛堣鍙栧姩浣滃彟鎸変笉瓒呰繃 60 绉掕锛?|

`uploadUrl` 涓?`baseUrl` 涓嶅悓鏄父瑙佹儏褰㈣€岄潪鐗逛緥锛氫笅杞借蛋 CDN 鎴栧叕缃戝煙鍚嶏紝涓婁紶璧板唴缃戝湴鍧€銆?

token 鍙粠鐜鍙橀噺璇伙紝**涓嶅啓杩涢厤缃枃浠?*锛屽洜涓洪厤缃槸瑕佹彁浜よ繘浠撳簱鐨勩€?

浜х墿涓婁紶鏄祦寮忕殑锛堟樉寮?`Content-Length`锛屼笉鏁磋杩涘唴瀛橈級锛屽洜涓轰竴涓骇鐗╁姩杈勬暟鐧?MB锛岃€屾墽琛屽彂甯冪殑寰€寰€鏄唴瀛樻湁闄愮殑 CI runner銆?

**PUT 涓嶈窡闅忛噸瀹氬悜銆?* 瀵逛笅杞芥潵璇撮噸瀹氬悜鏄父鎬侊紝涓斾簨鍚庢湁 sha256 鍏滃簳锛涘涓婁紶鏉ヨ閲嶅畾鍚戞剰鍛崇潃瀛楄妭钀藉埌浜嗛厤缃箣澶栫殑鍦版柟锛屼笖娌℃湁浠讳綍浜嬪悗妫€鏌ヨ兘鍙戠幇銆備笂浼犵鐐硅嫢杩佺Щ浜嗭紝搴斿綋鏀归厤缃紝鑰屼笉鏄宸ュ叿鍘昏拷銆?

璇诲彇涓€寰嬭蛋鐪熷疄 HTTP锛岃繖鏄畠涓?`local` 鐨勬牴鏈尯鍒細妫€鏌ユ湰鍦扮鐩樹笂鐨勫瓧鑺傚彧鑳借瘉鏄庡彂甯冩楠よ窇杩囦簡锛岃€屾姄鍙栧啓杩?manifest 鐨勯偅涓?URL 鎵嶈兘璇佹槑瀹㈡埛绔嬁寰楀埌銆傝姹傞伒瀹?SPEC.md 搂3.2 鐨勫鎴风瑙勫垯 鈥斺€?璺熼殢閲嶅畾鍚戜絾涓婇檺 5 璺炽€佹嫆缁?https 闄嶇骇涓?http銆佽 index 鏃堕檮甯︾紦瀛樺嚮绌垮弬鏁颁笌 `Cache-Control: no-cache`銆?

`verify --deep` 鐢?HEAD锛堟湇鍔″櫒涓嶆敮鎸佹椂閫€鍖栦负 1 瀛楄妭 Range 璇锋眰锛夋帰娴嬫瘡涓骇鐗╋紝**涓嶄笅杞戒骇鐗╂湰浣?*锛氫竴娆″彂甯冨姩杈勬暟鐧?MB锛屽叏閲忎笅杞戒細璁?`--deep` 鏄傝吹鍒版病浜烘効鎰忚窇锛岃€屾病浜鸿窇鐨勬鏌ョ瓑浜庝笉瀛樺湪銆傚畠鏍稿瀛樺湪鎬т笌澶у皬锛涘唴瀹圭敱 manifest 閲岀殑 `sha256` 淇濊瘉锛屾瘡涓鎴风涓嬭浇鏃堕兘浼氳嚜琛岄獙璇併€?

### 6.4 CNB 寰呴獙璇佷簨椤?

CNB 鐨勫埗鍝佸簱鍙湁鐢熸€佷笓鐢ㄧ被鍨嬶紙Docker / Helm / Maven / npm / PyPI / Cargo / Conan 绛夛級锛?*娌℃湁閫氱敤锛坮aw / generic锛夊埗鍝佸簱**锛岀ぞ鍖哄凡鎻愬嚭璇夋眰浣嗗皻鏈疄鐜般€傞€氱敤鏂囦欢鐨勫畼鏂硅矾寰勬槸 **Release 闄勪欢**锛坄cnbcool/attachments` 鎻掍欢鎴?Open API锛夛紝鑰屾枃妗ｄ腑鐨勪笅杞界ず渚嬪潎鎼哄甫 token銆?

鍥犳瀹炵幇 `cnb-release` 涔嬪墠**蹇呴』**鍏堢‘璁わ細鍏紑浠撳簱鐨?Release 闄勪欢鏄惁瀛樺湪绋冲畾鐨勩€佸尶鍚嶅彲璁块棶鐨?HTTP 鐩撮摼銆傞獙璇佹柟娉曪細鍦ㄤ竴涓叕寮€鐨?CNB 浠撳簱涓婁紶涓€涓檮浠讹紝鐒跺悗鍦?*鏈璇?*鐨勭幆澧冧笅锛堜笉甯︿换浣?token銆佷笉甯?cookie锛夎姹傚叾涓嬭浇鍦板潃锛岀‘璁よ繑鍥?200 涓斿唴瀹规纭紝骞惰褰曟槸鍚﹀彂鐢?302 璺宠浆銆?

鑻ョ粨璁烘槸蹇呴』閴存潈锛屽垯 CNB **涓嶈兘**浣滀负瀹㈡埛绔殑鐩存帴涓嬭浇婧愶紝鍙兘鐢ㄤ簬鍐呴儴鍒嗗彂锛涙鏃跺簲鏀圭敤 `s3-compatible` 鎴栬嚜寤洪潤鎬佹墭绠°€傝繖涓笉纭畾鎬т笉闃诲浠讳綍鍏朵粬宸ヤ綔 鈥斺€?鐢变簬 URL 涓€寰嬪啓鍦ㄧ鍚嶆枃妗ｉ噷锛屽悗绔崲鎴愪粈涔堥兘涓嶅奖鍝嶅凡鍙戝竷鐨勫鎴风銆?

**鍙︿竴鏉¤矾寰勫彲鑳借 `cnb-release` 瀹屽叏娌℃湁蹇呰锛?* 鑻?CNB 鐨?*浠撳簱鐩撮摼**锛坄/-/raw/<branch>/<path>` 褰㈠紡锛夊彲鍖垮悕璁块棶锛岀洿鎺ョ敤 `static-http` 鍗冲彲 鈥斺€?浜х墿鎻愪氦杩涗粨搴擄紝CI 澶╃劧瀹屾垚鍒嗗彂锛屾棤闇€浠讳綍涓婁紶瀹炵幇銆傝繖鏉¤矾鍊煎緱鍏堥獙锛屽洜涓哄畠鎴愭湰鏇翠綆涓斾笉渚濊禆 Release 闄勪欢鐨勯壌鏉冪瓥鐣ャ€備唬浠锋槸浜х墿浼氳繘鍏ヤ粨搴撳巻鍙诧紝闇€瑕佽瘎浼颁綋绉笌 LFS 閰嶇疆銆?

---



### 6.5 推荐对外入口（自有域名 + COS）

拓扑已冻结（详见设计文档）；`s3-compatible` **已实现**（SigV4）：

- 客户端 `entryUrls` 的主 URL 应为**自有域名绑定腾讯云 COS**（可选 CDN）上的固定绝对路径，例如 `https://updates.firoyang.com/rup/directory/<product>.pb`。
- 发布控制面在可信 CVM / CI（持钥签名）；写桶走 `s3-compatible`。COS **不是** `relkit-serve` 那种 Bearer 上传服务。
- `directory` / `index` / `fallback` / `manifest` / `artifact` 都是静态对象，**可以**整棵进 COS；镜像切换靠双写再改 directory / `urls[]`（见设计文档 §8）。
- 本仓库自用实装（桶 / DNS / CVM）见设计文档 §10。

权威说明：[`docs/design/update-ingress-cos.md`](docs/design/update-ingress-cos.md)。相关：[`docs/design/bootstrap-directory.md`](docs/design/bootstrap-directory.md)、ADR 0005。

---


### 6.6 `s3-compatible`

路径型后端：上传走 S3 兼容 API（SigV4），客户端下载仍只看 `baseUrl + key`（自有域名 / CDN）。适用于腾讯云 COS、AWS S3、MinIO。

```json
{
  "type": "s3-compatible",
  "endpoint": "https://cos.ap-guangzhou.myqcloud.com",
  "bucket": "relkit-updates-1251882798",
  "region": "ap-guangzhou",
  "prefix": "rup/",
  "baseUrl": "https://updates.firoyang.com/rup/",
  "accessKeyEnv": "COS_SECRET_ID",
  "secretKeyEnv": "COS_SECRET_KEY"
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `baseUrl` | 是 | 客户端下载基址；URL = `baseUrl + key` |
| `endpoint` | 是 | 对象存储 API 基址（不含 bucket） |
| `bucket` | 是 | 桶名 |
| `accessKeyEnv` / `secretKeyEnv` | 是 | 凭据所在环境变量名 |
| `prefix` | 否 | 对象 key 前缀，自动补末尾 `/` |
| `region` | 否 | 签名区域；COS 可从 `endpoint` 推导（如 `ap-guangzhou`） |
| `forcePathStyle` | 否 | 强制 path-style；自定义/本地 endpoint 默认开启，COS/AWS 默认 virtual-hosted |
| `timeoutSeconds` | 否 | 上传超时，默认 600 |

拓扑与缓存约定见 [`docs/design/update-ingress-cos.md`](docs/design/update-ingress-cos.md)。

---

### 6.7 `relkit-agent`（发布机）

CI 只 `stage`（staged 树含 `staged.pb`、`release-policy.json`、`artifacts/`），发布机持钥 `publish`。二进制：`cmd/relkit-agent`。运维命令与路径见 [`cmd/relkit-agent/README.md`](cmd/relkit-agent/README.md)。

- 发布配置：staged `release-policy.json` + `/etc/relkit-agent/products/<product>.json`（缺一不可）
- 迁 profile：`relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -migrate-profile`（迁完把产品根 `relkit.json` 改名为 `.migrated`）
- `PUT /v1/drop/{product}/{version}/{filename}` — 双 Job 交换 zip（Bearer；GET/HEAD 同样鉴权）
- `PUT /v1/staged/{product}/{version}` — staged 目录的 tar.gz（Bearer）
- `POST /v1/publish` — 触发 `publish.Run`（按 product 串行 + 幂等键）
- `GET /-/health`

部署样例与 Caddy 反代见 [`deploy/`](deploy/)。设计说明：[`docs/design/publish-agent.md`](docs/design/publish-agent.md)。

---

## 7. CI 集成

CI **不持**签名私钥，也 **不持** COS 写密钥。Runner 只做 `relkit stage`，把 staged 树打包后交给发布机上的 `relkit-agent`；agent 按产品 `signing.privateKeyPath` 持钥执行 `publish`。

```yaml
- name: Stage
  run: |
    relkit stage "${VERSION}" --code "${GITHUB_RUN_NUMBER}" \
      --min-from "${MIN_FROM}" \
      --add dist/app-win-x64.zip   os=windows,arch=x64 \
      --add dist/app-mac-arm64.zip os=macos,arch=arm64
    tar -C ".relkit/staged/${VERSION}" -czf staged.tar.gz .

- name: Publish via agent
  env:
    RELKIT_AGENT_TOKEN: ${{ secrets.RELKIT_AGENT_TOKEN }}
  run: |
    SHA=$(sha256sum staged.tar.gz | awk '{print $1}')
    curl -fsS -X PUT \
      -H "Authorization: Bearer ${RELKIT_AGENT_TOKEN}" \
      --data-binary @staged.tar.gz \
      "${RELKIT_AGENT_URL}/v1/staged/${PRODUCT}/${VERSION}"
    curl -fsS -X POST \
      -H "Authorization: Bearer ${RELKIT_AGENT_TOKEN}" \
      -H "Content-Type: application/json" \
      -d "{\"product\":\"${PRODUCT}\",\"version\":\"${VERSION}\",\"stagedSha256\":\"${SHA}\"}" \
      "${RELKIT_AGENT_URL}/v1/publish"
```

不要在 CI 里设置 `RELKIT_PRIVATE_KEY` 或任何签名私钥环境变量。设计说明：[`docs/design/publish-agent.md`](docs/design/publish-agent.md)。

---

## 8. 瀹炵幇浣嶇疆

姝ｅ紡浠ｇ爜鍦?[cnb.cool/shichao402/relkit](https://cnb.cool/shichao402/relkit)锛堝惈 `cmd/relkit` 涓?`cmd/relkit-serve`锛夛細

```
cmd/relkit/                 CLI 鍏ュ彛
internal/chain/             閫夎矾涓庡彲杈炬€э紙SPEC.md 搂9銆伮?0锛?
internal/selectors/         浜х墿閫夋嫨锛圫PEC.md 搂11锛?
internal/envelope/          绛惧悕淇″皝锛圫PEC.md 搂4锛?
internal/model/ jsonio/     瀵硅薄鏋勯€犮€佺‘瀹氭€у簭鍒楀寲
internal/config/ keys/      閰嶇疆涓庡瘑閽?
internal/stage/ publish/    鍥哄寲涓庡崄姝ュ彂甯冪紪鎺?
internal/verify/ simulate/  鏍￠獙涓庨€夎矾妯℃嫙
internal/httpx/ backends/   HTTP 涓?local / static-http / http-put / s3-compatible
e2e/                        绂荤嚎涓庣湡瀹?HTTP 绔埌绔?
testdata/conformance/       鍗忚澶瑰叿鍓湰锛堜笌鏈洰褰曞悓姝ワ級
```

`chain` / `selectors` / `envelope` 鏄鑼冩€ц涓虹殑鏉冨▉瀹炵幇锛宑onformance 娴嬭瘯鐩存帴鍔犺浇鏈洰褰曪紙鎴栦粨搴撳唴 `testdata/conformance`锛夌殑 JSON 澶瑰叿銆