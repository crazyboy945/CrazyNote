#!/usr/bin/env python3
"""把 crazynote 打包成 fnOS 可安装的 .fpk。

.fpk 容器结构（fnpack 1.2.3 产物格式）：
    manifest          应用清单
    app.tgz           app/ 目录的 gzip tar
    cmd/              生命周期脚本
    config/           privilege / resource
    wizard/           向导定义
    ICON.PNG          64x64
    ICON_256.PNG      256x256

外层为 gzip 压缩的 tar，manifest.checksum = app.tgz 字节的 MD5。
"""
import gzip
import hashlib
import io
import os
import shutil
import subprocess
import sys
import tarfile
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PKG = os.path.join(ROOT, 'packaging', 'crazynote')
DIST = os.path.join(ROOT, 'dist')
VERSION = os.environ.get('VERSION', '1.0.0')

# fnOS 二进制文件名后缀 → Go 交叉编译的 GOARCH
ARCH_MAP = {
    'x86_64': 'amd64',
    'aarch64': 'arm64',
    'armv7l': 'armv7',
}

# manifest 的 platform 字段取值 → 对应的二进制文件名后缀
# platform=x86 → x86_64；platform=arm → aarch64 + armv7l；platform=all → 全部
PLATFORM_ARCHES = {
    'x86': ['x86_64'],
    'arm': ['aarch64', 'armv7l'],
    'all': ['x86_64', 'aarch64', 'armv7l'],
}


def run(cmd, cwd=None):
    print('  $', ' '.join(cmd))
    r = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True)
    if r.returncode != 0:
        print(r.stdout)
        print(r.stderr, file=sys.stderr)
        raise SystemExit(f'命令失败: {" ".join(cmd)}')
    return r.stdout


def collect_binaries(stage_bin, arch_filter=None):
    """把各架构二进制放进 app/bin/，并生成与 fnOS 架构命名一致的软链。

    arch_filter 为 None 时打包所有已编译架构；否则只打包指定架构，
    与 manifest 的 arch 字段保持一致，避免包内容与声明架构不符。
    """
    os.makedirs(stage_bin, exist_ok=True)
    found = []
    for arch, goarch in ARCH_MAP.items():
        if arch_filter and arch not in arch_filter:
            continue
        src = os.path.join(DIST, f'crazynote-linux-{goarch}')
        if not os.path.exists(src):
            continue
        dst = os.path.join(stage_bin, f'crazynote-linux-{arch}')
        shutil.copy2(src, dst)
        os.chmod(dst, 0o755)
        found.append(arch)
    if not found:
        raise SystemExit('dist/ 下没有找到已编译的 Linux 二进制，请先执行 scripts/build.sh')
    # 默认入口软链，安装脚本按顺序探测
    link = os.path.join(stage_bin, 'crazynote')
    if os.path.islink(link) or os.path.exists(link):
        os.remove(link)
    os.symlink(f'crazynote-linux-{found[0]}', link)
    return found


def make_tgz(src_dir, out_path):
    """把目录打成 gzip tar（必须是 w:gz，飞牛 appcenter 按 .tgz 用 gzip 解压）。

    Windows 下 os.stat 没有 exec 位，直接 tf.add 会让 bin/ 下的二进制变成 0666，
    导致安装后 app 启动时 Permission denied。这里显式强制 bin/ 下文件 0755。
    """
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode='w:gz') as tf:
        src_dir = os.path.abspath(src_dir)
        for root, dirs, files in os.walk(src_dir):
            dirs.sort()
            rel_root = os.path.relpath(root, src_dir)
            # 必须统一成 Linux 风格的正斜杠，否则 fnOS 解不出 bin/crazynote
            arc_root = '' if rel_root == '.' else rel_root.replace(os.sep, '/')
            if arc_root:
                d = tarfile.TarInfo(arc_root + '/')
                d.type = tarfile.DIRTYPE
                d.mode = 0o755
                d.mtime = int(time.time())
                tf.addfile(d)
            for name in sorted(files):
                full = os.path.join(root, name)
                arcname = os.path.relpath(full, src_dir).replace(os.sep, '/')
                if os.path.islink(full):
                    l = tarfile.TarInfo(arcname)
                    l.type = tarfile.SYMTYPE
                    l.linkname = os.readlink(full)
                    l.mode = 0o755
                    l.mtime = int(time.time())
                    tf.addfile(l)
                    continue
                with open(full, 'rb') as fh:
                    data = fh.read()
                info = tarfile.TarInfo(arcname)
                info.size = len(data)
                # bin/ 下二进制必须可执行（Windows 下 stat 无 exec 位，强制）
                info.mode = 0o755 if arcname.startswith('bin/') else 0o644
                info.mtime = int(time.time())
                tf.addfile(info, io.BytesIO(data))
    data = buf.getvalue()
    with open(out_path, 'wb') as f:
        f.write(data)
    return data


def read_manifest():
    path = os.path.join(PKG, 'manifest')
    items = []
    with open(path, encoding='utf-8') as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith('#'):
                continue
            k, _, v = line.partition('=')
            items.append((k.strip(), v.strip()))
    return items


def apply_version(items, version, changelog=''):
    """把 manifest 的 version / changelog 覆盖为本次构建的版本信息。

    fnOS 应用中心用 manifest 里的 version 判断能否覆盖安装：版本号不更新
    会被判成「已安装更高版本」而拒绝安装。所以这里强制与构建版本保持一致。
    """
    out = []
    has_version = False
    has_changelog = False
    for k, v in items:
        if k == 'version':
            out.append((k, version))
            has_version = True
        elif k == 'changelog' and changelog:
            out.append((k, changelog))
            has_changelog = True
        else:
            out.append((k, v))
    # 缺字段时补上（version 紧跟 appname）
    if not has_version:
        pos = 1 if out and out[0][0] == 'appname' else 0
        out.insert(pos, ('version', version))
    if changelog and not has_changelog:
        out.append(('changelog', changelog))
    return out


def build():
    print('==> 准备打包目录')
    stage = os.path.join(DIST, '_fpk_stage')
    if os.path.exists(stage):
        shutil.rmtree(stage)
    os.makedirs(stage)

    app_src = os.path.join(PKG, 'app')
    app_stage = os.path.join(stage, '_app')
    shutil.copytree(app_src, app_stage)

    # 读取 manifest 声明的 platform 字段，只打包对应架构的二进制。
    # platform 取值 x86|arm|all；缺失时兼容旧的 arch 字段。
    manifest_items = read_manifest()
    # manifest 的 version 必须跟随本次构建版本，否则 fnOS 会拒绝覆盖安装
    changelog = os.environ.get('CHANGELOG', '')
    manifest_items = apply_version(manifest_items, VERSION, changelog)
    manifest_dict = dict(manifest_items)
    platform = manifest_dict.get('platform', '').strip()
    arch_filter = None
    if platform in PLATFORM_ARCHES:
        arch_filter = PLATFORM_ARCHES[platform]
    else:
        # 兼容旧 manifest 的 arch=x86_64 / aarch64 / armv7l 写法
        declared_arch = manifest_dict.get('arch', '').strip()
        if declared_arch and declared_arch in ARCH_MAP:
            arch_filter = [declared_arch]

    print('==> 收集可执行文件')
    arches = collect_binaries(os.path.join(app_stage, 'bin'), arch_filter)
    print(f'    架构: {", ".join(arches)}')

    # 图标
    for src, dst_name in (('extension/icon.png', 'icon256.png'),):
        src_path = os.path.join(ROOT, src)
        if os.path.exists(src_path):
            shutil.copy2(src_path, os.path.join(stage, dst_name))
    # 64x64 图标：生成
    icon_script = os.path.join(ROOT, 'extension', 'make_icons.py')
    if os.path.exists(icon_script):
        run([sys.executable, icon_script], cwd=os.path.dirname(icon_script))
    icon64 = os.path.join(ROOT, 'extension', 'icons', 'icon64.png')
    icon256 = os.path.join(ROOT, 'extension', 'icons', 'icon256.png')
    if not os.path.exists(icon64):
        icon64 = os.path.join(ROOT, 'extension', 'icons', 'icon48.png')
    if not os.path.exists(icon256):
        icon256 = os.path.join(ROOT, 'extension', 'icons', 'icon128.png')
    if os.path.exists(icon64):
        shutil.copy2(icon64, os.path.join(stage, 'ICON.PNG'))
        shutil.copy2(icon256, os.path.join(stage, 'ICON_256.PNG'))

    print('==> 打包 app.tgz')
    app_tgz = os.path.join(stage, 'app.tgz')
    app_bytes = make_tgz(app_stage, app_tgz)
    checksum = hashlib.md5(app_bytes).hexdigest()
    shutil.rmtree(app_stage)

    print('==> 组装 .fpk')
    fpk_path = os.path.join(DIST, f'crazynote-{VERSION}.fpk')

    manifest_text = '\n'.join(f'{k}={v}' for k, v in manifest_items)
    manifest_text += f'\nchecksum={checksum}\n'
    manifest_bytes = manifest_text.encode('utf-8')

    with open(fpk_path, 'wb') as raw:
        with gzip.GzipFile(fileobj=raw, mode='wb', mtime=0) as gz:
            with tarfile.open(fileobj=gz, mode='w') as tf:
                def add_bytes(name, data, mode=0o644):
                    info = tarfile.TarInfo(name)
                    info.size = len(data)
                    info.mode = mode
                    info.mtime = int(time.time())
                    tf.addfile(info, io.BytesIO(data))

                def add_file(name, path, mode=0o644):
                    with open(path, 'rb') as f:
                        add_bytes(name, f.read(), mode)

                add_bytes('manifest', manifest_bytes)
                add_file('app.tgz', app_tgz)
                for sub in ('cmd', 'config', 'wizard'):
                    d = os.path.join(PKG, sub)
                    if not os.path.isdir(d):
                        continue
                    for name in sorted(os.listdir(d)):
                        p = os.path.join(d, name)
                        if os.path.isfile(p):
                            add_file(f'{sub}/{name}', p, 0o755 if sub == 'cmd' else 0o644)
                for name in ('ICON.PNG', 'ICON_256.PNG'):
                    p = os.path.join(stage, name)
                    if os.path.exists(p):
                        add_file(name, p)

    shutil.rmtree(stage)
    size = os.path.getsize(fpk_path)
    sha = hashlib.sha256(open(fpk_path, 'rb').read()).hexdigest()
    print(f'\n✅ FPK 生成: {fpk_path}')
    print(f'   大小: {size/1024:.1f} KB')
    print(f'   SHA256: {sha}')
    print(f'   manifest.checksum = {checksum}')


if __name__ == '__main__':
    build()
