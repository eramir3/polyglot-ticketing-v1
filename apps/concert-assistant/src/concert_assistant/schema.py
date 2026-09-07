"""Curated schema metadata exposed to the text-to-SQL planner."""

from __future__ import annotations


COLUMN_DESCRIPTIONS = {
    "id": "unique concert record identifier",
    "date": "concert start date",
    "end_date": "concert end date when known",
    "venue": "venue name",
    "city": "city name",
    "country": "country name",
    "artist": "performing artist name",
    "is_festival": "whether the concert was a festival",
    "end_date_parse_error": "source end-date parsing issue, if any",
    "concerts_last_year": "artist concert count in the preceding year",
    "concerts_last_3_years": "artist concert count in the preceding three years",
    "concerts_last_5_years": "artist concert count in the preceding five years",
    "days_since_last_global_concert": "days since the artist's preceding concert globally",
    "activity_trend": "derived artist activity trend",
    "artist_expansion_ratio": "derived artist geographic expansion ratio",
    "artist_years_observed": "years the artist is observed in the dataset",
    "artist_concerts_last_30_days": "artist concerts in the preceding 30 days",
    "artist_concerts_last_90_days": "artist concerts in the preceding 90 days",
    "is_currently_touring": "whether the artist was currently touring",
    "days_since_previous_concert": "days since the previous artist concert",
    "distinct_countries_last_30_days": "countries visited by the artist in the preceding 30 days",
    "previous_visits_to_country": "previous visits by the artist to this country",
    "is_first_observed_visit": "whether this is the first observed artist visit to the country",
    "days_since_last_country_visit": "days since the artist last visited this country",
    "country_average_gap": "artist's average gap between visits to this country",
    "country_median_gap": "artist's median gap between visits to this country",
    "country_gap_std": "artist's standard deviation of country-visit gaps",
    "country_min_gap": "artist's minimum gap between visits to this country",
    "country_max_gap": "artist's maximum gap between visits to this country",
    "country_last_gap": "artist's most recent gap between visits to this country",
    "country_gap_trend": "derived trend of country-visit gaps",
    "country_gap_coefficient_of_variation": "variation coefficient of country-visit gaps",
    "same_season_country_visit_ratio": "ratio of visits in the same season for this country",
    "country_seasonal_concentration": "seasonal concentration of country visits",
    "country_visit_proportion": "proportion of the artist's concerts in this country",
    "country_rank_for_artist": "country rank by the artist's concert activity",
    "unique_countries_visited_total": "total countries visited by the artist",
    "unique_countries_visited_last_3_years": "countries visited in the preceding three years",
    "unique_countries_visited_last_5_years": "countries visited in the preceding five years",
    "unique_cities_last_5_years": "cities visited in the preceding five years",
    "country_concert_ratio_last_5_years": "country concert ratio in the preceding five years",
    "total_previous_concerts_in_country": "previous artist concerts in this country",
    "global_average_country_gap": "artist's global average gap between country visits",
    "season": "season of the concert date",
    "is_post_covid": "whether the concert is after the COVID period",
    "is_covid_period": "whether the concert occurred during the COVID period",
}

ALLOWED_COLUMNS = frozenset(COLUMN_DESCRIPTIONS)


def schema_prompt() -> str:
    columns = "\n".join(f"- {name}: {description}" for name, description in COLUMN_DESCRIPTIONS.items())
    return f"""The only available relation is public.concerts. It contains one row per concert.

Allowed columns:
{columns}
"""
