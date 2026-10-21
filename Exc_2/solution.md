postgres=# DROP TABLE IF EXISTS orders;
NOTICE:  table "orders" does not exist, skipping
DROP TABLE
Time: 0.754 ms
postgres=# CREATE TABLE orders (
postgres(# customer_name TEXT,
postgres(# product_category TEXT,
postgres(# quantity INTEGER,
postgres(# price_per_unit DECIMAL(10,2),
postgres(# order_date DATE,
postgres(# country TEXT
postgres(# );
CREATE TABLE
Time: 4.799 ms
postgres=# \COPY orders FROM '/data/orders_1M.csv' DELIMITER ',' CSV HEADER;
COPY 1000000
Time: 517.070 ms
postgres=# SELECT *
postgres-# FROM orders
postgres-# WHERE price_per_unit = (SELECT MAX(price_per_unit) FROM orders);
Time: 72.521 ms
postgres=# SELECT product_category, SUM(quantity) AS total_quantity
postgres-# FROM orders
postgres-# WHERE 1 = 1
postgres-# GROUP BY product_category
postgres-# ORDER BY total_quantity DESC
postgres-# LIMIT 3;
 product_category | total_quantity 
------------------+----------------
 Health & Beauty  |         300842
 Electronics      |         300804
 Toys             |         300598
(3 rows)

Time: 121.816 ms
postgres=# SELECT product_category, SUM(price_per_unit * quantity) AS total_revenue
postgres-# FROM orders
postgres-# WHERE 1 = 1
postgres-# GROUP BY product_category;
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
postgres=# SELECT customer_name, SUM(quantity * price_per_unit) AS total_spent, COUNT(*) AS order_count
postgres-# FROM orders
postgres-# WHERE 1 = 1
postgres-# GROUP BY customer_name
postgres-# ORDER BY total_spent
postgres-# LIMIT 5;
 customer_name | total_spent | order_count 
---------------+-------------+-------------
 James Perez   |   618676.28 |         842
 Olivia Clark  |   658863.41 |         928
 Olivia Davis  |   664810.12 |         888
 Anna Miller   |   673542.01 |         876
 Ivy Perez     |   674494.68 |         894
(5 rows)

Time: 213.279 ms
postgres=# SELECT customer_name, SUM(quantity * price_per_unit) AS total_spent, COUNT(*) AS order_count
postgres-# FROM orders
postgres-# WHERE 1 = 1
postgres-# GROUP BY customer_name
postgres-# ORDER BY total_spent DESC
postgres-# LIMIT 5;
 customer_name  | total_spent | order_count 
----------------+-------------+-------------
 Carol Taylor   |   991179.18 |        1028
 Nina Lopez     |   975444.95 |         980
 Daniel Jackson |   959344.48 |        1033
 Carol Lewis    |   947708.57 |         943
 Daniel Young   |   946030.14 |         973
(5 rows)

Time: 156.268 ms
postgres=# 
-------
All top 5 customers have nearly identical order counts (~950 to 1,030 orders) and remarkably similar spending totals (~$940k to $990k). Even comparing the highest spenders to the lowest spenders (~800 to 900 orders), there are no extreme single-purchase buyers and no extreme "whales." The orders and spending are distributed almost evenly across all customers.
Why the data looks like this (from dataset_generator.py):
Limited Combination Pool: There are 32 FIRST_NAMES and 33 LAST_NAMES, producing exactly 32×33=1,056 unique customer combinations.
Uniform Random Sampling: The script uses random.choice(FIRST_NAMES) and random.choice(LAST_NAMES) inside random_order() across 1,000,000 generated rows.
Law of Large Numbers: Because every order draws a name uniformly at random, the expected number of orders per customer is:
1,056
1,000,000
​	
 ≈947 orders per customer
The slight variations in order count (~800–1,030) and total spending represent standard random variance around the mean, not deliberate customer segmentation or real-world Pareto/Zipfian buying behavior.
--------------
