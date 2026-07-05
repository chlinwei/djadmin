import warnings

from cryptography.utils import CryptographyDeprecationWarning


def silence_known_warnings():
	warnings.filterwarnings(
		'ignore',
		message='.*TripleDES has been moved to cryptography\\.hazmat\\.decrepit\\.ciphers\\.algorithms\\.TripleDES.*',
		category=CryptographyDeprecationWarning,
	)


silence_known_warnings()

from .celery import app as celery_app

__all__ = ('celery_app',)
