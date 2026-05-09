from pathlib import Path
import shutil
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parent
SIZES = [1024, 512, 256, 128, 64, 32, 16]


def icon_svg(size: int) -> str:
    inset = max(1, int(size * 0.095))
    mark_size = size - inset * 2
    scale = mark_size / 256
    return f"""<svg xmlns="http://www.w3.org/2000/svg" width="{size}" height="{size}" viewBox="0 0 {size} {size}" fill="none">
  <g transform="translate({inset} {inset}) scale({scale})">
    <rect width="256" height="256" rx="64" fill="#111214"/>
    <path d="M83 94.5L117.5 60H177V94H132L101 125L132 156H177V190H117.5L83 155.5C66.2 138.7 66.2 111.3 83 94.5Z" fill="#FBFBFA"/>
    <path d="M149 122H194V154H149V122Z" fill="#FBFBFA"/>
  </g>
</svg>
"""


def main() -> None:
    renderer = shutil.which("rsvg-convert")
    if not renderer:
        raise SystemExit("rsvg-convert is required to regenerate app icons")

    with tempfile.TemporaryDirectory() as temp_dir:
        temp_path = Path(temp_dir)
        for size in SIZES:
            svg_path = temp_path / f"app-icon-{size}.svg"
            svg_path.write_text(icon_svg(size), encoding="utf-8")
            subprocess.run(
                [
                    renderer,
                    "-w",
                    str(size),
                    "-h",
                    str(size),
                    "-o",
                    str(ROOT / f"app-icon-{size}.png"),
                    str(svg_path),
                ],
                check=True,
            )

    print("icons done")


if __name__ == "__main__":
    main()
