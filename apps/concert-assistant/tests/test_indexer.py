from datetime import date

import pytest

from concert_assistant.indexer import ArtistProfile, build_profile, validate_embeddings


def test_builds_a_deterministic_touring_profile() -> None:
    profile = build_profile(
        {
            "artist": "Radiohead",
            "concert_count": 12,
            "first_concert": date(2015, 1, 1),
            "last_concert": date(2020, 12, 31),
            "country_count": 4,
            "city_count": 8,
            "festival_count": 3,
            "countries": "France, Germany, Spain, United Kingdom",
            "seasons": "fall, spring, summer",
            "average_gap_days": 42.5,
            "average_concerts_last_year": 15.2,
            "observed_currently_touring": True,
        }
    )

    assert profile.artist == "Radiohead"
    assert "12 concerts from 2015-01-01 to 2020-12-31" in profile.profile_text
    assert "Festival appearances: 3 (25%" in profile.profile_text
    assert len(profile.source_fingerprint) == 64


def test_rejects_embeddings_with_an_unexpected_dimension() -> None:
    profile = ArtistProfile("Radiohead", "profile", "fingerprint")

    with pytest.raises(RuntimeError, match="768-dimension"):
        validate_embeddings([profile], [[0.0] * 3], "nomic-embed-text")
