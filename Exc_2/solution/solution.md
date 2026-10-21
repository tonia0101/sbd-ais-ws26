# Activity 2.1 — PostgreSQL Analytical Queries (E-commerce)

## Table creation
```sql
DROP TABLE IF EXISTS orders;

CREATE TABLE orders (
  customer_name TEXT,
  product_category TEXT,
  quantity INTEGER,
  price_per_unit DECIMAL(10, 2),
  order_date DATE,
  country TEXT
);

\COPY orders FROM '/data/orders_1M.csv' DELIMITER ',' CSV HEADER;
```
## A. Order with highest price_per_unit
```sql
SELECT *
FROM orders
WHERE price_per_unit = (SELECT MAX(price_per_unit) FROM orders);
```
customer_name | product_category | quantity | price_per_unit | order_date | country 
---------------+------------------+----------+----------------+------------+---------
 Emma Brown    | Automotive       |        3 |        2000.00 | 2024-10-11 | Italy
(1 row)

## B. Top 3 product_categories with highest total quantity sold across all orders
```sql
SELECT product_category, SUM(quantity) AS total_quantity
FROM orders
WHERE 1 = 1
GROUP BY product_category
ORDER BY total_quantity DESC
LIMIT 3;
```

product_category | total_quantity 
------------------+----------------
 Health & Beauty  |         300842
 Electronics      |         300804
 Toys             |         300598
(3 rows)

Time: 121.816 ms

## C. Total revenue per product category
```sql
SELECT product_category, SUM(price_per_unit * quantity) AS total_revenue
FROM orders
WHERE 1 = 1
GROUP BY product_category;
```
 product_category | total_revenue 
------------------+---------------
 Automotive       |  306589798.86
 Books            |   12731976.04
 Electronics      |  241525009.45
 Fashion          |   31566368.22
 Grocery          |   15268355.66
 Health & Beauty  |   46599817.89
 Home & Garden    |   78023780.09
 Office Supplies  |   38276061.64
 Sports           |   61848990.83
 Toys             |   23271039.02
(10 rows)

Time: 169.953 ms

## D. Top 5 customers by total spending
```sql
SELECT customer_name, SUM(quantity * price_per_unit) AS total_spent, COUNT(*) AS order_count
FROM orders
WHERE 1 = 1
GROUP BY customer_name
ORDER BY total_spent DESC
LIMIT 5;
```


 customer_name  | total_spent | order_count 
----------------+-------------+-------------
 Carol Taylor   |   991179.18 |        1028
 Nina Lopez     |   975444.95 |         980
 Daniel Jackson |   959344.48 |        1033
 Carol Lewis    |   947708.57 |         943
 Daniel Young   |   946030.14 |         973
(5 rows)

Time: 156.268 ms

## E. Looking at spending totals and order counts 
The top 5 spenders each accumulated nearly $1,000,000 in spending across approximately 950 to 1,030 orders each. Across the entire dataset, even the lowest spending customers have ~800 to 900 orders. There are no outliers. 

- The python script generates names by combining 32 FIRST_NAMES and 33 LAST_NAMES, yielding exactly 32×33=1,056 unique customer names.
- For each of the 1,000,000 generated records, names are selected via random.choice(). (uniform random sampling)
- Because the sampling is uniform across 1,000,000 rows, every customer is expected to receive: 1,000,000 / 1,056 = 947 orders. The variation between customers represents standard variance from indipendent uniform sampling.

# Activity 2.2

## 1. Measure how it grows

```sql
CREATE TABLE people_50k  AS SELECT * FROM people_big WHERE id <= 50000;
CREATE TABLE people_100k AS SELECT * FROM people_big WHERE id <= 100000;
CREATE TABLE people_200k AS SELECT * FROM people_big WHERE id <= 200000;
```
SELECT 50000
Time: 33.920 ms
SELECT 100000
Time: 37.988 ms
SELECT 200000
Time: 56.573 ms


| rows in table | join result (COUNT(*)) | time |
| :--- | :--- | :--- |
| 50 000 | 27501822 | 919.346 ms |
| 100 000 | 109946508 | 3203.778 ms (00:03.204) |
| 200 000 | 439395606 | 12432.351 ms (00:12.432) |

- from 50k to 100k: results (3,998x), time (3,48x)
- from 100k to 200k: result (3,996x), time (3,88x)

Prediction for people_big: 
- 1,000,000 is 10x the size of people_100k under quadratic_growth O(N^2) --> result size 10^2 = 100x
- predicted runtime: 3.20 s×100=320 seconds (~5.3 minutes)

## 2. Does an Index help? 
```sql
CREATE INDEX idx_country_100k ON people_100k(country);
ANALYZE people_100k;

EXPLAIN ANALYZE
SELECT COUNT(*)
FROM people_100k p1
JOIN people_100k p2 ON p1.country = p2.country;
```
CREATE INDEX
Time: 74.026 ms
ANALYZE
Time: 52.271 ms
Time: 7362.515 ms (00:07.363)

No it did not improve. It took around 7 seconds which is slower than the unindexed run.

When we look at EXPLAIN ANALYZE the database did use the index. Because we are joining the entire table against itself, the database still has to load all 100,000 rows into memory and match them up. still has to generate and count 100,000*100,000 combinations

## 3. Rewrite the Query 
```sql
WITH country_counts AS (
    SELECT COUNT(*) AS cnt
    FROM people_100k
    GROUP BY country
)
SELECT SUM(cnt::bigint * cnt) AS total_pairs
FROM country_counts;
```
 total_pairs 
   109946508
(1 row)

Time: 11.267 ms

The rewritten query returned exactly 109,946,508, which perfectly matches the result of the slow join on the people_100k table. When I ran the rewrite on the full 1-million row people_big table, it finished in just 222.979 milliseconds. This beats my prediction from Step 1 (which estimated roughly 320 seconds), proving that changing the algorithmic complexity from O(N^2) to O(N) is massively faster.

## 4. Discussion 
Writing a better query O(N) is much more effective than using faster hardware or indexes for a bad query O(N^2). Better hardware only hides the problem for a little bit.

If the company really needs all 11 billion rows we either need to scale-up (use a bigger machine) or Scale-out (Cluster). Even with these solutions it might still be a problem due to the size of the data.

Limits of OLTP database for this database: PostgreSQL is an OLTP database. It is built for quick, everyday transactions and not meant for such big calculations. If we run huge self-joins on the main database, it uses up all the CPU and slows down the system for regular users. For this kind of heavy analytical work, we should copy the data to a specialized OLAP database (like Snowflake) instead.
