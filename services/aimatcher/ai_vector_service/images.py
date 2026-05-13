from io import BytesIO
from typing import Protocol

from PIL import Image, ImageOps, UnidentifiedImageError


class ImageDecodeError(ValueError):
    pass


class ImageDecoder(Protocol):
    def decode(self, data: bytes) -> Image.Image:
        pass


class PillowImageDecoder:
    def __init__(
        self,
        max_image_pixels: int = 4096 * 4096,
        target_size: tuple[int, int] = (320, 320),
    ) -> None:
        if max_image_pixels <= 0:
            raise ValueError("max_image_pixels должен быть положительным")
        if target_size[0] <= 0 or target_size[1] <= 0:
            raise ValueError("размеры target_size должны быть положительными")

        self._max_image_pixels = max_image_pixels
        self._target_size = target_size
        Image.MAX_IMAGE_PIXELS = max_image_pixels

    def decode(self, data: bytes) -> Image.Image:
        if not data:
            raise ImageDecodeError("данные изображения пустые")

        try:
            with Image.open(BytesIO(data)) as image:
                width, height = image.size
                if width <= 0 or height <= 0:
                    raise ImageDecodeError("размеры изображения некорректны")
                if width * height > self._max_image_pixels:
                    raise ImageDecodeError("количество пикселей изображения превышает лимит")

                rgb_image = image.convert("RGB")
                return ImageOps.contain(
                    rgb_image,
                    self._target_size,
                    method=Image.Resampling.LANCZOS,
                )
        except ImageDecodeError:
            raise
        except Image.DecompressionBombError as err:
            raise ImageDecodeError("количество пикселей изображения превышает лимит") from err
        except (UnidentifiedImageError, OSError) as err:
            raise ImageDecodeError("данные не являются поддерживаемым изображением") from err
