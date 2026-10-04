# Demo photos (Daily Bean tenant)

Uploaded to R2 on Oct 4, 2026 via `POST /admin/uploads` and attached with `PUT /admin/menu/items/:id`.
All from Wikimedia Commons; CC BY / BY-SA photos need the credit below if shown publicly.

| Món | Ảnh | Giấy phép | Tác giả |
|---|---|---|---|
| Cà phê sữa đá | [Ca Phe Sua Da.jpg](https://commons.wikimedia.org/wiki/File%3ACa_Phe_Sua_Da.jpg) | CC BY 2.5 | <a href="//commons.wikimedia.org/wiki/User:Clarin" title="Us |
| Bạc xỉu | [Iced Bạc Xỉu served in tall glass on the table in tha cafe, Danang city, 2023.jpg](https://commons.wikimedia.org/wiki/File%3AIced_B%E1%BA%A1c_X%E1%BB%89u_served_in_tall_glass_on_the_table_in_tha_cafe%2C_Danang_city%2C_2023.jpg) | CC BY-SA 4.0 | <a href="//commons.wikimedia.org/w/index.php?title=User:Klie |
| Cà phê đen | [Vietnamese coffee with milk and ice.jpg](https://commons.wikimedia.org/wiki/File%3AVietnamese_coffee_with_milk_and_ice.jpg) | CC BY-SA 2.0 | ePi.Longo |
| Trà đào cam sả | [Peach iced tea with orange slices.jpg](https://commons.wikimedia.org/wiki/File%3APeach_iced_tea_with_orange_slices.jpg) | CC BY-SA 4.0 | <a href="//commons.wikimedia.org/w/index.php?title=User:Baoo |
| Trà vải | [Iced lychee tea and Es teh manis - Bali 2025-09-25.jpg](https://commons.wikimedia.org/wiki/File%3AIced_lychee_tea_and_Es_teh_manis_-_Bali_2025-09-25.jpg) | CC0 | <a href="//commons.wikimedia.org/wiki/User:Onthewings" title |
| Cookies & Cream | [Oreo milkshake - Creperie Doux Sourire 2025-05-03.jpg](https://commons.wikimedia.org/wiki/File%3AOreo_milkshake_-_Creperie_Doux_Sourire_2025-05-03.jpg) | CC0 | <a href="//commons.wikimedia.org/wiki/User:Onthewings" title |
| Bánh croissant | [Croissants au beurre (18953292873).jpg](https://commons.wikimedia.org/wiki/File%3ACroissants_au_beurre_%2818953292873%29.jpg) | CC0 | Herry Wibisono (<a rel="nofollow" class="external text" href |
| Espresso | [Cup of espresso 02.jpg](https://commons.wikimedia.org/wiki/File%3ACup_of_espresso_02.jpg) | CC BY-SA 4.0 | <a href="//commons.wikimedia.org/wiki/User:Kritzolina" title |

Brand logo: `dailybean-logo.svg` drawn for the demo (terracotta circle, cream bean, DAILY BEAN wordmark).

Note: the R2 API token used by the backend cannot set bucket CORS, so `cdn.coffeesos.online` sends no
`Access-Control-Allow-Origin`. The Flutter web app falls back to an `<img>` element
(`webHtmlElementStrategy: fallback`); add a CORS policy on the bucket in the Cloudflare dashboard
(R2 → coffeesos → Settings → CORS, allow GET from `*`) to let CanvasKit decode the images directly.
