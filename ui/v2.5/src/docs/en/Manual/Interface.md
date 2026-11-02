# Interface options

## Language

Setting the language affects the formatting of numbers and dates, and the names shown in Country/Region fields. Country and region names come from Unicode CLDR and are the same in every browser. Hong Kong, Macao and Palestine use CLDR’s short forms.

## SFW content mode

SFW content mode is used to indicate that the content being managed is _not_ adult content. 

When SFW content mode is enabled, the following changes are made to the UI:
- default performer images are changed to less adult-oriented images
- certain adult-specific metadata fields are hidden (e.g. performer genital fields)
- `O`-Counter is replaced with `Like`-counter

## Scene/marker wall preview type

The Scene Wall and Marker pages display scene preview videos (mp4) by default. This can be changed to animated image (webp) or static image. 

> **⚠️ Note:** scene/marker preview videos must be generated to see them in the applicable wall page if Video preview type is selected. Likewise, if Animated image is selected, then Image Previews must be generated.

## Show studio overlay as text

By default, in the grid card view the studio will be shown as an image overlay of the studio logo. Checking this option changes this to display studios as a text name instead.

## Scene player options

By default, scene videos do not automatically start when navigating to the scenes page. Checking the "Auto-start video" option changes this to auto play scene videos.

The maximum loop duration option allows looping of shorter videos. Set this value to the maximum scene duration that scene videos should loop. Setting this to 0 disables this functionality.

### Activity tracking

The "Track Activity" option allows tracking of scene play count and duration, and sets the resume point when a scene video is not finished.

The "Minimum play percent" gives the minimum proportion of a video that must be played before the play count of the scene is incremented.

By default, when a scene has a resume point, the scene player will automatically seek to this point when the scene is played. Setting "Always start video from beginning" to true disables this behaviour.

## Custom CSS

The stash UI can be customised using custom CSS. See [here](https://discourse.stashapp.cc/t/custom-css-snippets/4043) for a community-curated set of CSS snippets to customise your UI. 

There is also a [collection of community-created themes](https://discourse.stashapp.cc/tags/c/plugins/18/all/theme) available.

## Custom JavaScript

Stash supports the injection of custom JavaScript to assist with theming or adding additional functionality. Be aware that bad JavaScript could break the UI or worse.

## Custom locales

The localisation strings can be customised. The master list of default (en-GB) locale strings can be found [here](https://github.com/stashapp/stash/blob/develop/ui/v2.5/src/locales/en-GB.json). The custom locale format is the same as this json file.

For example, to override the `actions.add_directory` label (which is `Add directory` by default), you would have the following in the custom locale:

```
{
  "actions": {
    "add_directory": "Some other description"
  }
}
```

Country and region names can also be customised in Interface settings under Custom Localisation. Enable custom localisation and add `country_names` entries using uppercase two-letter ISO country codes. For example, `country_names.TW` can be set with:

```json
{
  "country_names": {
    "TW": "My preferred name"
  }
}
```

Countries and regions without an override keep their CLDR names. Overrides change displayed names, including Country/Region selections and filters; selections are sorted by displayed name in the UI language. Stored country codes are unchanged.

## Custom served folders

It is possible to expose specific folders to the UI. This configuration is performed manually in the `config.yml` file only.

Custom served content is exposed via the `/custom` URL path prefix.

For example, in the `config.yml` file:
```
custom_served_folders:
  /: D:\stash\static
  /foo: D:\bar
```

With the above configuration, a request for `/custom/foo/bar.png` would return `D:\bar\bar.png`. The `/` entry matches anything that is not otherwise mapped by the other entries. For example, `/custom/baz/xyz.png` would return `D:\stash\static\baz\xyz.png`.

Applications for this include using static images in custom css, like the Plex theme. For example, using the following config:
```yml
custom_served_folders:
  /: <stash folder>\custom
```

The `background.png` and `noise.png` files can be placed in the `custom` folder, then in the custom css, the `./background.png` and `./noise.png` strings can be replaced with `/custom/background.png` and `/custom/noise.png` respectively.

Other applications are to add custom UIs to stash, accessible via `/custom`.

## Country and region name data

Country and region names come from Unicode CLDR, used under the Unicode License V3:

Copyright © 2001–2026 Unicode, Inc.

NOTICE TO USER: Carefully read the following legal agreement. BY
DOWNLOADING, INSTALLING, COPYING OR OTHERWISE USING DATA FILES, AND/OR
SOFTWARE, YOU UNEQUIVOCALLY ACCEPT, AND AGREE TO BE BOUND BY, ALL OF THE
TERMS AND CONDITIONS OF THIS AGREEMENT. IF YOU DO NOT AGREE, DO NOT
DOWNLOAD, INSTALL, COPY, DISTRIBUTE OR USE THE DATA FILES OR SOFTWARE.

Permission is hereby granted, free of charge, to any person obtaining a
copy of data files and any associated documentation (the "Data Files") or
software and any associated documentation (the "Software") to deal in the
Data Files or Software without restriction, including without limitation
the rights to use, copy, modify, merge, publish, distribute, and/or sell
copies of the Data Files or Software, and to permit persons to whom the
Data Files or Software are furnished to do so, provided that either (a)
this copyright and permission notice appear with all copies of the Data
Files or Software, or (b) this copyright and permission notice appear in
associated Documentation.

THE DATA FILES AND SOFTWARE ARE PROVIDED "AS IS", WITHOUT WARRANTY OF ANY
KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT OF
THIRD PARTY RIGHTS.

IN NO EVENT SHALL THE COPYRIGHT HOLDER OR HOLDERS INCLUDED IN THIS NOTICE
BE LIABLE FOR ANY CLAIM, OR ANY SPECIAL INDIRECT OR CONSEQUENTIAL DAMAGES,
OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS,
WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION,
ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THE DATA
FILES OR SOFTWARE.

Except as contained in this notice, the name of a copyright holder shall
not be used in advertising or otherwise to promote the sale, use or other
dealings in these Data Files or Software without prior written
authorization of the copyright holder.
