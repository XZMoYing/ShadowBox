# Third-party software

ShadowBox is distributed under GPL-3.0-or-later.

The proxy engine is built directly from MatsuriDayo/sing-box commit
aed32ee3066cdbc7d471e3e0415c5134088962df, version 1.12.19-neko-1,
as pinned by NekoBox for Android release 1.4.2.

- https://github.com/MatsuriDayo/sing-box
- https://github.com/MatsuriDayo/NekoBoxForAndroid
- Copyright (C) 2022 by nekohasekai; GPL-3.0-or-later.
- The upstream engine license additionally restricts using its application name
  or implying association without prior consent. ShadowBox has independent branding.

Profile format interpretation is based on the NekoBox 1.4.2 source, including
its AbstractBean, UniversalFmt, protocol beans, and configuration semantics.
The Java/Kotlin files in nekobox-reference/ are unmodified references and are not
compiled into this application. Original upstream copyrights and GPL terms apply.
Where source files lack individual notices, refer to the upstream root LICENSE.

The visual system follows Material 3 guidance. FlClash is a design reference;
ShadowBox does not include its Clash.Meta engine or branding.

Flutter/Dart and official Flutter packages: BSD licenses, with their bundled notices.
Rust tray-icon, tao, dirs, fs2, serde, serde_json, and ureq retain their respective
MIT/Apache/BSD licenses. Go dependencies retain their respective licenses. Exact
versions and checksums are recorded in Cargo.lock, go.mod, go.sum, and pubspec.lock.

External protocol executables are supplied separately by the user; they are not
part of this package and retain their own licenses.
